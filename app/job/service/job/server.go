package job

import (
	"context"
	"fmt"
	stdlog "log"
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
	"github.com/segmentio/kafka-go"
)

var _ transport.Server = (*Server)(nil)
var _ event2.Message = (*Message)(nil)

type Server struct {
	reader *kafka.Reader
	topic  string
	uo     *service.OrderService
	rdb    *redis.Client
	log    *klog.Helper
	tasks  []scheduledTask
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
	go func() {
		for {
			m, err := s.reader.FetchMessage(context.Background())

			if err != nil {
				break
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
				stdlog.Fatal("message handing exception:", err)
			}
			if err := s.reader.CommitMessages(ctx, m); err != nil {
				stdlog.Fatal("failed to commit message:", err)
			}
		}
	}()
	return nil
}

func (s Server) Close() error {
	err := s.reader.Close()
	if err != nil {
		return err
	}
	return nil
}

func NewJOBServer(_ *conf.Server, data *conf.Data, uo *service.OrderService, logger klog.Logger) *Server {

	// []string{"192.168.2.27:9092"}, "order"

	address := envutil.CSV("KAFKA_BROKERS", []string{"192.168.0.111:9092"})
	topic := "order"

	r := kafka.NewReader(kafka.ReaderConfig{
		Brokers:  address,
		GroupID:  "group-d",
		Topic:    topic,
		MinBytes: 1,    // 10kb
		MaxBytes: 10e6, // 10mb
	})
	rdb := redis.NewClient(&redis.Options{
		Addr:         data.Redis.Addr,
		WriteTimeout: data.Redis.WriteTimeout.AsDuration(),
		ReadTimeout:  data.Redis.ReadTimeout.AsDuration(),
	})
	s := &Server{
		reader: r,
		topic:  topic,
		uo:     uo,
		rdb:    rdb,
		log:    klog.NewHelper(klog.With(logger, "module", "job/server")),
	}
	s.tasks = []scheduledTask{
		{
			name:     "order_sync_repair",
			interval: 5 * time.Second,
			run: func(ctx context.Context, _ time.Time) (string, error) {
				repaired, err := s.uo.RepairPending(ctx, 20)
				return fmt.Sprintf("repaired=%d", repaired), err
			},
		},
		{
			name:     "order_timeout_close",
			interval: 5 * time.Second,
			run: func(ctx context.Context, now time.Time) (string, error) {
				closed, err := s.uo.CloseExpired(ctx, now, 20)
				return fmt.Sprintf("closed=%d", closed), err
			},
		},
		{
			name:     "stock_consistency_check",
			interval: 10 * time.Second,
			run: func(ctx context.Context, _ time.Time) (string, error) {
				mismatches, err := s.uo.CheckStockConsistency(ctx, 20)
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
	return s
}

func (s Server) Start(ctx context.Context) error {
	fmt.Printf("job-job start")

	//in := &v1.HelloRequest{
	//	Name: "kratos",
	//}
	//s.t.SayHello(ctx, in)

	s.Receive(ctx, func(ctx context.Context, message event2.Message) error {
		//TODO::路由解析 根据不同的key调用不同的业务逻辑处理

		msg := message.Header()

		//fmt.Println(msg["uid"])

		uid, err := strconv.ParseInt(msg["uid"], 10, 64)
		gid, err := strconv.ParseInt(msg["goods_id"], 10, 64)
		oid, err := strconv.ParseInt(msg["order_id"], 10, 64)
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
		s.uo.Create(ctx, in)
		fmt.Printf("key:%s, value:%s, header:%s\n", message.Key(), message.Value(), message.Header())
		return nil
	})

	for _, task := range s.tasks {
		go s.runTaskLoop(ctx, task)
	}

	return nil
}

func (s Server) Stop(ctx context.Context) error {
	err := s.reader.Close()
	if err != nil {
		return err
	}
	return nil
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
				s.log.Errorf("task run failed: task=%s err=%v", task.name, runErr)
			} else {
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
