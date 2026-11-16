package providerhealth

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"io"
	"net"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/egose/aiproxy/internal/config"
)

func readHealthTestRedisCommand(reader *bufio.Reader) ([]string, error) {
	line, err := reader.ReadString('\n')
	if err != nil || !strings.HasPrefix(line, "*") {
		return nil, fmt.Errorf("invalid Redis test command: %w", err)
	}
	count, err := strconv.Atoi(strings.TrimSpace(line[1:]))
	if err != nil || count < 1 || count > 32 {
		return nil, errors.New("invalid Redis test argument count")
	}
	args := make([]string, count)
	for index := range args {
		line, err := reader.ReadString('\n')
		if err != nil || !strings.HasPrefix(line, "$") {
			return nil, errors.New("invalid Redis test argument")
		}
		size, err := strconv.Atoi(strings.TrimSpace(line[1:]))
		if err != nil || size < 0 || size > 4096 {
			return nil, errors.New("invalid Redis test argument size")
		}
		data := make([]byte, size+2)
		if _, err := io.ReadFull(reader, data); err != nil {
			return nil, err
		}
		args[index] = string(data[:size])
	}
	return args, nil
}

func TestRedisPersistenceHonorsCompletionDeadlineDuringRealSilentRead(t *testing.T) {
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	entered, stop, accepted := make(chan struct{}, 1), make(chan struct{}), make(chan struct{})
	var mu sync.Mutex
	var connections []net.Conn
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
				reader := bufio.NewReader(conn)
				for {
					args, err := readHealthTestRedisCommand(reader)
					if err != nil {
						return
					}
					switch strings.ToLower(args[0]) {
					case "hello":
						_, _ = io.WriteString(conn, "-ERR unknown command 'hello'\r\n")
					case "del":
						select {
						case entered <- struct{}{}:
						default:
						}
						<-stop
						return
					default:
						_, _ = io.WriteString(conn, "+OK\r\n")
					}
				}
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
	backend, err := newRedisBackend("redis://"+listener.Addr().String(), "test")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = backend.Close() })
	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()
	begin := time.Now()
	err = backend.MarkSuccess(ctx, "provider")
	if err == nil || time.Since(begin) > time.Second || !backend.client.Options().ContextTimeoutEnabled {
		t.Fatalf("Redis sink ignored the completion deadline: %v (%v)", err, time.Since(begin))
	}
	select {
	case <-entered:
	default:
		t.Fatal("test did not exercise a real stalled Redis persistence command")
	}
	select {
	case <-ctx.Done():
	case <-time.After(time.Second):
		t.Fatal("Redis deadline did not expire")
	}
	if !errors.Is(ctx.Err(), context.DeadlineExceeded) {
		t.Fatal("Redis persistence failed before exercising the completion deadline")
	}
}

func TestLocalCompletionHealthSurvivesExpiredAndFailedRemotePersistence(t *testing.T) {
	var calls atomic.Int32
	tracker := NewWithBackend(nil, config.ProviderHealth{}, stubBackend{
		markFailure: func(ctx context.Context, _ string, _ time.Duration) error {
			calls.Add(1)
			if _, ok := ctx.Deadline(); !ok {
				t.Error("remote health persistence lost its deadline")
			}
			return errors.New("remote unavailable")
		},
		isHealthy: func(context.Context, string) (bool, error) { return true, errors.New("remote unavailable") },
	})
	tracker.RecordLocal("provider", false)
	if calls.Load() != 0 {
		t.Fatal("in-memory completion synchronously persisted remote health")
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if err := tracker.PersistContext(ctx, "provider", false); !errors.Is(err, context.Canceled) || calls.Load() != 0 {
		t.Fatal("expired queued health record reached the backend")
	}
	if tracker.IsHealthy("provider") {
		t.Fatal("failed/expired persistence erased known local health")
	}
	active, cancelActive := context.WithTimeout(context.Background(), time.Second)
	defer cancelActive()
	if err := tracker.PersistContext(active, "provider", false); err == nil || calls.Load() != 1 || tracker.IsHealthy("provider") {
		t.Fatal("remote persistence failure retried accounting or replaced local completion state")
	}
}
