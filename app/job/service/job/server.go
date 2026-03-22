package job

import (
	"context"
	"fmt"
	"math/rand"
	"strconv"
	"time"

	klog "github.com/go-kratos/kratos/v2/log"
	"github.com/go-kratos/kratos/v2/transport"
	"github.com/go-redis/redis/v8"
	event2 "github.com/littleSand/adama/app/job/service/event"
	"github.com/littleSand/adama/app/job/service/internal/biz"
	"github.com/littleSand/adama/app/job/service/internal/conf"
	"github.com/littleSand/adama/app/job/service/internal/service"
	"github.com/littleSand/adama/pkg/envutil"
	"github.com/littleSand/adama/pkg/poolutil"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/segmentio/kafka-go"
)

var _ transport.Server = (*Server)(nil)
var _ event2.Message = (*Message)(nil)

var taskRunCounter = prometheus.NewCounterVec(
	prometheus.CounterOpts{
		Namespace: "adama",
		Subsystem: "task",
		Name:      "job_runs_total",
		Help:      "Total number of scheduled task runs partitioned by task name and result.",
	},
	[]string{"task", "result"},
)

func init() {
	prometheus.MustRegister(taskRunCounter)
}

type Server struct {
	role           string
	readers        []*kafka.Reader
	retryWriter    *kafka.Writer
	dlqWriter      *kafka.Writer
	topic          string
	dlqTopic       string
	uo             *service.OrderService
	rdb            *redis.Client
	log            *klog.Helper
	tasks          []scheduledTask
	workerCount    int
	queueSize      int
	maxRetry       int
	publishTimeout time.Duration
	workQueue      chan consumeJob
}

type scheduledTask struct {
	name     string
	interval time.Duration
	run      func(context.Context, time.Time) (string, error)
}

type Message struct {
	key    string
	value  []byte
	header map[string]string
}

type consumeJob struct {
	reader  *kafka.Reader
	message kafka.Message
	payload *Message
}

const (
	headerRetryCount    = "x-retry-count"
	headerFailedReason  = "x-failed-reason"
	headerOriginalTopic = "x-original-topic"
)

func (m *Message) Key() string {
	return m.key
}

func (m *Message) Value() []byte {
	return m.value
}

func (m *Message) Header() map[string]string {
	return m.header
}

func NewMessage(key string, value []byte, header map[string]string) event2.Message {
	return &Message{
		key:    key,
		value:  value,
		header: header,
	}
}

func (s Server) Receive(ctx context.Context, handler event2.Handler) error {
	if len(s.readers) == 0 {
		return nil
	}
	for i := 0; i < s.workerCount; i++ {
		go s.workerLoop(ctx, handler, i)
	}
	for _, reader := range s.readers {
		go s.consumeLoop(ctx, reader)
	}
	return nil
}

func (s Server) Close() error {
	var firstErr error
	for _, reader := range s.readers {
		if err := reader.Close(); err != nil && firstErr == nil {
			firstErr = err
		}
	}
	if s.retryWriter != nil {
		if err := s.retryWriter.Close(); err != nil && firstErr == nil {
			firstErr = err
		}
	}
	if s.dlqWriter != nil {
		if err := s.dlqWriter.Close(); err != nil && firstErr == nil {
			firstErr = err
		}
	}
	return firstErr
}

func NewJOBServer(_ *conf.Server, data *conf.Data, uo *service.OrderService, logger klog.Logger) *Server {
	role := envutil.Get("JOB_ROLE", "worker")
	address := envutil.CSV("KAFKA_BROKERS", []string{"192.168.0.111:9092"})
	topic := "order"

	var readers []*kafka.Reader
	var retryWriter *kafka.Writer
	var dlqWriter *kafka.Writer
	dlqTopic := envutil.Get("TASK_KAFKA_DLQ_TOPIC", topic+"-dlq")
	if role == "worker" {
		consumerCount := envutil.Int("TASK_KAFKA_CONSUMERS", 8)
		if consumerCount <= 0 {
			consumerCount = 1
		}
		if err := ensureKafkaTopic(address, dlqTopic, envutil.Int("TASK_KAFKA_DLQ_PARTITIONS", 1)); err != nil {
			klog.NewHelper(klog.With(logger, "module", "job/server", "role", role)).Warnf("ensure dlq topic failed: topic=%s err=%v", dlqTopic, err)
		}
		readers = make([]*kafka.Reader, 0, consumerCount)
		for i := 0; i < consumerCount; i++ {
			readers = append(readers, kafka.NewReader(kafka.ReaderConfig{
				Brokers:        address,
				GroupID:        "group-d",
				Topic:          topic,
				MinBytes:       1,
				MaxBytes:       10e6,
				CommitInterval: time.Second,
				MaxWait:        100 * time.Millisecond,
			}))
		}
		retryWriter = newKafkaWriter(address, topic)
		dlqWriter = newKafkaWriter(address, dlqTopic)
	}

	redisOptions := &redis.Options{
		Addr:         data.Redis.Addr,
		WriteTimeout: data.Redis.WriteTimeout.AsDuration(),
		ReadTimeout:  data.Redis.ReadTimeout.AsDuration(),
	}
	poolutil.ConfigureRedisOptions(redisOptions, "TASK")
	rdb := redis.NewClient(redisOptions)
	s := &Server{
		role:           role,
		readers:        readers,
		retryWriter:    retryWriter,
		dlqWriter:      dlqWriter,
		topic:          topic,
		dlqTopic:       dlqTopic,
		uo:             uo,
		rdb:            rdb,
		log:            klog.NewHelper(klog.With(logger, "module", "job/server", "role", role)),
		workerCount:    envutil.Int("TASK_KAFKA_WORKERS", 12),
		queueSize:      envutil.Int("TASK_KAFKA_QUEUE_SIZE", 1280),
		maxRetry:       envutil.Int("TASK_KAFKA_MAX_RETRIES", 3),
		publishTimeout: envutil.Duration("TASK_KAFKA_PUBLISH_TIMEOUT", 3*time.Second),
	}
	if s.maxRetry < 0 {
		s.maxRetry = 0
	}
	if s.publishTimeout <= 0 {
		s.publishTimeout = 3 * time.Second
	}
	if s.workerCount <= 0 {
		s.workerCount = len(readers)
		if s.workerCount <= 0 {
			s.workerCount = 1
		}
	}
	if s.queueSize <= 0 {
		s.queueSize = s.workerCount * 64
	}
	if len(readers) > 0 {
		s.workQueue = make(chan consumeJob, s.queueSize)
	}
	s.tasks = buildTasksByRole(s)
	return s
}

func (s Server) Start(ctx context.Context) error {
	s.log.Infof("job server start role=%s readers=%d workers=%d queue=%d", s.role, len(s.readers), s.workerCount, s.queueSize)

	if len(s.readers) > 0 {
		if err := s.Receive(ctx, func(ctx context.Context, message event2.Message) error {
			msg := message.Header()

			uid, err := strconv.ParseInt(msg["uid"], 10, 64)
			if err != nil {
				return err
			}
			gid, err := strconv.ParseInt(msg["goods_id"], 10, 64)
			if err != nil {
				return err
			}
			oid, err := strconv.ParseInt(msg["order_id"], 10, 64)
			if err != nil {
				return err
			}
			amount, err := strconv.ParseInt(msg["amount"], 10, 64)
			if err != nil {
				return err
			}
			expireAt, err := time.Parse(time.RFC3339, msg["expire_at"])
			if err != nil {
				return err
			}

			in := &biz.AdamaOrder{
				OrderId:    oid,
				UserId:     uid,
				GoodsId:    gid,
				Amount:     amount,
				StockToken: msg["stock_token"],
				ExpireAt:   expireAt,
			}
			return s.uo.Create(ctx, in)
		}); err != nil {
			return err
		}
	}

	for _, task := range s.tasks {
		go s.runTaskLoop(ctx, task)
	}

	return nil
}

func (s Server) Stop(ctx context.Context) error {
	return s.Close()
}

func (s Server) runTaskLoop(ctx context.Context, task scheduledTask) {
	ticker := time.NewTicker(task.interval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case now := <-ticker.C:
			token := fmt.Sprintf("%d-%d", time.Now().UnixNano(), rand.Int63())
			locked, err := s.acquireTaskLock(ctx, task.name, token, task.interval)
			if err != nil {
				s.log.Errorf("task lock failed: task=%s err=%v", task.name, err)
				continue
			}
			if !locked {
				continue
			}
			result, runErr := task.run(ctx, now)
			if runErr != nil {
				taskRunCounter.WithLabelValues(task.name, "failed").Inc()
				s.log.Errorf("task run failed: task=%s err=%v", task.name, runErr)
			} else {
				taskRunCounter.WithLabelValues(task.name, "success").Inc()
				s.log.Infof("task run finished: task=%s %s", task.name, result)
			}
			_ = s.releaseTaskLock(ctx, task.name, token)
		}
	}
}

func (s Server) acquireTaskLock(ctx context.Context, taskName, token string, interval time.Duration) (bool, error) {
	lockTTL := interval + 2*time.Second
	return s.rdb.SetNX(ctx, "JOB:TASK:LOCK:"+taskName, token, lockTTL).Result()
}

func (s Server) releaseTaskLock(ctx context.Context, taskName, token string) error {
	const script = `
if redis.call("GET", KEYS[1]) == ARGV[1] then
	return redis.call("DEL", KEYS[1])
end
return 0
`
	return s.rdb.Eval(ctx, script, []string{"JOB:TASK:LOCK:" + taskName}, token).Err()
}

func (s Server) consumeLoop(ctx context.Context, reader *kafka.Reader) {
	for {
		m, err := reader.FetchMessage(ctx)
		if err != nil {
			if ctx.Err() != nil {
				return
			}
			s.log.Errorf("kafka fetch failed: topic=%s err=%v", s.topic, err)
			time.Sleep(500 * time.Millisecond)
			continue
		}
		h := make(map[string]string)
		if len(m.Headers) > 0 {
			for _, header := range m.Headers {
				h[header.Key] = string(header.Value)
			}
		}
		job := consumeJob{
			reader:  reader,
			message: m,
			payload: &Message{
				key:    string(m.Key),
				value:  m.Value,
				header: h,
			},
		}
		select {
		case <-ctx.Done():
			return
		case s.workQueue <- job:
		}
	}
}

func (s Server) workerLoop(ctx context.Context, handler event2.Handler, workerID int) {
	for {
		select {
		case <-ctx.Done():
			return
		case job := <-s.workQueue:
			err := handler(ctx, job.payload)
			if err != nil {
				retryCount := parseRetryCount(job.payload.header)
				nextRetry := retryCount + 1
				if nextRetry > s.maxRetry {
					if dlqErr := s.publishFailure(ctx, s.dlqWriter, job, nextRetry, err); dlqErr != nil {
						s.log.Errorf("message dlq publish failed: worker=%d topic=%s partition=%d offset=%d retries=%d err=%v", workerID, job.message.Topic, job.message.Partition, job.message.Offset, nextRetry, dlqErr)
						time.Sleep(200 * time.Millisecond)
						continue
					}
					s.log.Warnf("message moved to dlq: worker=%d topic=%s partition=%d offset=%d retries=%d err=%v", workerID, job.message.Topic, job.message.Partition, job.message.Offset, retryCount, err)
					if err := job.reader.CommitMessages(ctx, job.message); err != nil {
						s.log.Errorf("failed to commit dlq message: worker=%d topic=%s partition=%d offset=%d err=%v", workerID, job.message.Topic, job.message.Partition, job.message.Offset, err)
					}
					continue
				}
				if retryErr := s.publishFailure(ctx, s.retryWriter, job, nextRetry, err); retryErr != nil {
					s.log.Errorf("message retry publish failed: worker=%d topic=%s partition=%d offset=%d retries=%d err=%v", workerID, job.message.Topic, job.message.Partition, job.message.Offset, nextRetry, retryErr)
					time.Sleep(200 * time.Millisecond)
					continue
				}
				s.log.Warnf("message requeued: worker=%d topic=%s partition=%d offset=%d retries=%d err=%v", workerID, job.message.Topic, job.message.Partition, job.message.Offset, nextRetry, err)
				if err := job.reader.CommitMessages(ctx, job.message); err != nil {
					s.log.Errorf("failed to commit retried message: worker=%d topic=%s partition=%d offset=%d err=%v", workerID, job.message.Topic, job.message.Partition, job.message.Offset, err)
				}
				continue
			}
			if err := job.reader.CommitMessages(ctx, job.message); err != nil {
				s.log.Errorf("failed to commit message: worker=%d topic=%s partition=%d offset=%d err=%v", workerID, job.message.Topic, job.message.Partition, job.message.Offset, err)
			}
		}
	}
}

func (s Server) publishFailure(ctx context.Context, writer *kafka.Writer, job consumeJob, retryCount int, handleErr error) error {
	if writer == nil {
		return fmt.Errorf("kafka writer not configured")
	}
	headers := make([]kafka.Header, 0, len(job.message.Headers)+3)
	for _, header := range job.message.Headers {
		if header.Key == headerRetryCount || header.Key == headerFailedReason || header.Key == headerOriginalTopic {
			continue
		}
		headers = append(headers, header)
	}
	headers = append(headers,
		kafka.Header{Key: headerRetryCount, Value: []byte(strconv.Itoa(retryCount))},
		kafka.Header{Key: headerFailedReason, Value: []byte(trimFailureReason(handleErr))},
		kafka.Header{Key: headerOriginalTopic, Value: []byte(job.message.Topic)},
	)
	publishCtx, cancel := context.WithTimeout(context.Background(), s.publishTimeout)
	defer cancel()
	return writer.WriteMessages(publishCtx, kafka.Message{
		Key:     job.message.Key,
		Value:   job.message.Value,
		Headers: headers,
	})
}

func parseRetryCount(headers map[string]string) int {
	if len(headers) == 0 {
		return 0
	}
	raw := headers[headerRetryCount]
	if raw == "" {
		return 0
	}
	value, err := strconv.Atoi(raw)
	if err != nil || value < 0 {
		return 0
	}
	return value
}

func trimFailureReason(err error) string {
	if err == nil {
		return ""
	}
	text := err.Error()
	if len(text) > 200 {
		return text[:200]
	}
	return text
}

func newKafkaWriter(address []string, topic string) *kafka.Writer {
	return &kafka.Writer{
		Topic:        topic,
		Addr:         kafka.TCP(address...),
		Balancer:     &kafka.LeastBytes{},
		BatchTimeout: 5 * time.Millisecond,
		BatchSize:    1,
		RequiredAcks: kafka.RequireOne,
		Async:        false,
	}
}

func ensureKafkaTopic(address []string, topic string, partitions int) error {
	if len(address) == 0 || topic == "" {
		return nil
	}
	if partitions <= 0 {
		partitions = 1
	}

	conn, err := kafka.Dial("tcp", address[0])
	if err != nil {
		return err
	}
	defer conn.Close()

	controller, err := conn.Controller()
	if err != nil {
		return err
	}
	controllerConn, err := kafka.Dial("tcp", fmt.Sprintf("%s:%d", controller.Host, controller.Port))
	if err != nil {
		return err
	}
	defer controllerConn.Close()

	err = controllerConn.CreateTopics(kafka.TopicConfig{
		Topic:             topic,
		NumPartitions:     partitions,
		ReplicationFactor: 1,
	})
	if err != nil && err != kafka.TopicAlreadyExists {
		return err
	}
	return nil
}

func buildTasksByRole(s *Server) []scheduledTask {
	switch s.role {
	case "timeout":
		return []scheduledTask{
			{
				name:     "order_timeout_close",
				interval: 5 * time.Second,
				run: func(ctx context.Context, now time.Time) (string, error) {
					closed, err := s.uo.CloseExpired(ctx, now, envutil.Int("TASK_TIMEOUT_CLOSE_LIMIT", 200))
					return fmt.Sprintf("closed=%d", closed), err
				},
			},
		}
	case "scheduler":
		return []scheduledTask{
			{
				name:     "order_sync_repair",
				interval: 5 * time.Second,
				run: func(ctx context.Context, _ time.Time) (string, error) {
					repaired, err := s.uo.RepairPending(ctx, envutil.Int("TASK_REPAIR_LIMIT", 100))
					return fmt.Sprintf("repaired=%d", repaired), err
				},
			},
			{
				name:     "stock_consistency_check",
				interval: 10 * time.Second,
				run: func(ctx context.Context, _ time.Time) (string, error) {
					mismatches, err := s.uo.CheckStockConsistency(ctx, envutil.Int("TASK_STOCK_CHECK_LIMIT", 100))
					return fmt.Sprintf("mismatches=%d", mismatches), err
				},
			},
			{
				name:     "cleanup_stats",
				interval: 15 * time.Second,
				run: func(ctx context.Context, _ time.Time) (string, error) {
					stats, err := s.uo.CollectWorkflowStats(ctx)
					return fmt.Sprintf("stats=%v", stats), err
				},
			},
		}
	default:
		return nil
	}
}
