package mongolog

import (
	"context"
	"errors"
	"net"
	"sync"
	"testing"
	"time"

	"github.com/egose/aiproxy/internal/payloadlog"
	"go.mongodb.org/mongo-driver/v2/mongo"
	"go.mongodb.org/mongo-driver/v2/mongo/options"
)

func TestRecordContextHonorsCompletionDeadlineAgainstSilentMongoEndpoint(t *testing.T) {
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	var mu sync.Mutex
	var connections []net.Conn
	started, stop, accepted := make(chan struct{}, 1), make(chan struct{}), make(chan struct{})
	var handlers sync.WaitGroup
	handlers.Add(1)
	go func() {
		defer handlers.Done()
		defer close(accepted)
		for {
			conn, err := listener.Accept()
			if err != nil {
				return
			}
			mu.Lock()
			connections = append(connections, conn)
			mu.Unlock()
			handlers.Add(1)
			go func() {
				defer handlers.Done()
				var buffer [1024]byte
				if n, _ := conn.Read(buffer[:]); n > 0 {
					select {
					case started <- struct{}{}:
					default:
					}
				}
				<-stop
			}()
		}
	}()
	t.Cleanup(func() {
		_ = listener.Close()
		<-accepted
		close(stop)
		mu.Lock()
		for _, conn := range connections {
			_ = conn.Close()
		}
		mu.Unlock()
		handlers.Wait()
	})
	client, err := mongo.Connect(options.Client().ApplyURI("mongodb://" + listener.Addr().String()).
		SetConnectTimeout(3 * time.Second).SetServerSelectionTimeout(3 * time.Second).SetMaxPoolSize(1))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		ctx, cancel := context.WithTimeout(context.Background(), time.Second)
		defer cancel()
		if err := client.Disconnect(ctx); err != nil {
			t.Error(err)
		}
	})
	logger := &Logger{client: client, coll: client.Database("test").Collection("payloads"), timeout: 2 * time.Second}
	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()
	begin := time.Now()
	err = logger.RecordContext(ctx, payloadlog.Entry{RequestID: "once", Status: 200})
	if !errors.Is(err, context.DeadlineExceeded) || time.Since(begin) > time.Second {
		t.Fatalf("Mongo sink ignored the shorter completion deadline: %v (%v)", err, time.Since(begin))
	}
	select {
	case <-started:
	default:
		t.Fatal("test did not exercise a real silent Mongo handshake")
	}
	expired := &Logger{timeout: time.Second}
	if err := expired.RecordContext(ctx, payloadlog.Entry{RequestID: "expired"}); !errors.Is(err, context.DeadlineExceeded) {
		t.Fatal("expired queued Mongo work reached encoding/persistence")
	}
}
