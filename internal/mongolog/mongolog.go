package mongolog

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/egose/aiproxy/internal/config"
	"github.com/egose/aiproxy/internal/payloadlog"

	"go.mongodb.org/mongo-driver/v2/bson"
	"go.mongodb.org/mongo-driver/v2/mongo"
	"go.mongodb.org/mongo-driver/v2/mongo/options"
	"go.mongodb.org/mongo-driver/v2/mongo/readpref"
)

type Options struct {
	URI          string
	Database     string
	Collection   string
	MaxBodyBytes int
	Timeout      time.Duration
}

func OptionsFromConfig(cfg config.PayloadLog) Options {
	return Options{
		URI:          cfg.Mongo.URI,
		Database:     cfg.Mongo.Database,
		Collection:   cfg.Mongo.Collection,
		MaxBodyBytes: cfg.MaxBodyBytes,
		Timeout:      cfg.Mongo.Timeout,
	}
}

func (o Options) withDefaults() Options {
	if o.Database == "" {
		o.Database = config.DefaultPayloadMongoDatabase
	}
	if o.Collection == "" {
		o.Collection = config.DefaultPayloadMongoCollection
	}
	if o.Timeout <= 0 {
		o.Timeout = config.DefaultPayloadMongoTimeout
	}
	return o
}

type Logger struct {
	client  *mongo.Client
	coll    *mongo.Collection
	maxBody int
	timeout time.Duration
}

func New(opts Options) (*Logger, error) {
	if opts.URI == "" {
		return nil, nil
	}
	opts = opts.withDefaults()
	client, err := mongo.Connect(options.Client().ApplyURI(opts.URI))
	if err != nil {
		return nil, fmt.Errorf("mongodb connect: %w", err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), opts.Timeout)
	defer cancel()
	if err := client.Ping(ctx, readpref.Primary()); err != nil {
		_ = client.Disconnect(context.Background())
		return nil, fmt.Errorf("mongodb ping: %w", err)
	}
	return &Logger{
		client:  client,
		coll:    client.Database(opts.Database).Collection(opts.Collection),
		maxBody: opts.MaxBodyBytes,
		timeout: opts.Timeout,
	}, nil
}

func (l *Logger) MaxBodyBytes() int {
	if l == nil {
		return 0
	}
	return l.maxBody
}

func (l *Logger) Record(e payloadlog.Entry) error {
	if l == nil {
		return nil
	}
	if e.Timestamp == "" {
		e.Timestamp = time.Now().UTC().Format(time.RFC3339Nano)
	}
	doc, err := DocumentFor(e)
	if err != nil {
		return err
	}
	ctx, cancel := context.WithTimeout(context.Background(), l.timeout)
	defer cancel()
	_, err = l.coll.InsertOne(ctx, doc)
	if err != nil {
		return fmt.Errorf("mongodb insert: %w", err)
	}
	return nil
}

func (l *Logger) Close() error {
	if l == nil || l.client == nil {
		return nil
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	return l.client.Disconnect(ctx)
}

func DocumentFor(e payloadlog.Entry) (bson.M, error) {
	data, err := json.Marshal(e)
	if err != nil {
		return nil, err
	}
	var doc bson.M
	if err := bson.UnmarshalExtJSON(data, false, &doc); err != nil {
		return nil, err
	}
	return doc, nil
}
