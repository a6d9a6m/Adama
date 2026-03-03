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
	role    string
	readers []*kafka.Reader
	topic   string
	uo      *service.OrderService
	rdb     *redis.Client
	log     *klog.Helper
	tasks   []scheduledTask
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
	for _, reader := range s.readers {
		go s.consumeLoop(ctx, reader, handler)
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
	return firstErr
}

func NewJOBServer(_ *conf.Server, data *conf.Data, uo *service.OrderService, logger klog.Logger) *Server {
	role := envutil.Get("JOB_ROLE", "worker")
	address := envutil.CSV("KAFKA_BROKERS", []string{"192.168.0.111:9092"})
	topic := "order"

	var readers []*kafka.Reader
	if role == "worker" {
		consumerCount := envutil.Int("TASK_KAFKA_CONSUMERS", 8)
		if consumerCount <= 0 {
			consumerCount = 1
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
	}

	rdb := redis.NewClient(&redis.Options{
		Addr:         data.Redis.Addr,
		WriteTimeout: data.Redis.WriteTimeout.AsDuration(),
		ReadTimeout:  data.Redis.ReadTimeout.AsDuration(),
	})
	s := &Server{
		role:    role,
		readers: readers,
		topic:   topic,
		uo:      uo,
		rdb:     rdb,
		log:     klog.NewHelper(klog.With(logger, "module", "job/server", "role", role)),
	}
	s.tasks = buildTasksByRole(s)
	return s
}

func (s Server) Start(ctx context.Context) error {
	fmt.Printf("job-job start role=%s", s.role)

	if len(s.readers) > 0 {
		s.Receive(ctx, func(ctx context.Context, message event2.Message) error {
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

			in := &biz.AdamaOrder{
				OrderId:    oid,
				UserId:     uid,
				GoodsId:    gid,
				Amount:     amount,
				StockToken: msg["stock_token"],
			}
			return s.uo.Create(ctx, in)
		})
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

func (s Server) consumeLoop(ctx context.Context, reader *kafka.Reader, handler event2.Handler) {
	for {
		m, err := reader.FetchMessage(context.Background())
		if err != nil {
			s.log.Errorf("kafka fetch failed: topic=%s err=%v", s.topic, err)
			return
		}
		h := make(map[string]string)
		if len(m.Headers) > 0 {
			for _, header := range m.Headers {
				h[header.Key] = string(header.Value)
			}
		}
		err = handler(context.Background(), &Message{
			key:    string(m.Key),
			value:  m.Value,
			header: h,
		})
		if err != nil {
			s.log.Errorf("message handling failed: topic=%s partition=%d offset=%d err=%v", m.Topic, m.Partition, m.Offset, err)
		}
		if err := reader.CommitMessages(ctx, m); err != nil {
			s.log.Errorf("failed to commit message: topic=%s partition=%d offset=%d err=%v", m.Topic, m.Partition, m.Offset, err)
		}
	}
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
