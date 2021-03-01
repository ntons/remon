package remon

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/go-redis/redis/v8"
	"github.com/vmihailenco/msgpack/v4"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
)

type SyncClient struct {
	opts *xOptions
	rdb  RedisClient
	mdb  *mongo.Client
	// life-time control
	ctx  context.Context
	stop context.CancelFunc
	// rate limit
	counter int32
	cond    *sync.Cond
}

func NewSyncClient(
	rdb RedisClient, mdb *mongo.Client, opts ...Option) *SyncClient {
	o := newOptions()
	for _, opt := range opts {
		opt.apply(o)
	}
	ctx, stop := context.WithCancel(context.Background())
	return &SyncClient{opts: o, rdb: rdb, mdb: mdb, ctx: ctx, stop: stop}
}
func NewSync(rdb RedisClient, mdb *mongo.Client, opts ...Option) *SyncClient {
	return NewSyncClient(rdb, mdb, opts...)
}

func (cli *SyncClient) Serve() {
	var tick *time.Ticker // rate limit beat generater
	if cli.opts.syncRate > 0 {
		tick = time.NewTicker(time.Second / time.Duration(cli.opts.syncRate))
		defer tick.Stop()
	}
	for {
		key, data, err := cli.peek()
		for ; err == nil; key, data, err = cli.next(key, data.Rev) {
			if err = cli.save(key, data); err != nil {
				break
			}
			cli.opts.log.Debugf("sync: %s saved", key)
			if tick != nil {
				select {
				case <-cli.ctx.Done():
					return
				case <-tick.C:
				}
			}
		}
		if err != redis.Nil {
			cli.opts.log.Errorf("failed to sync: %v", err)
		}
		// no dirty data or other error, halt 1 second
		select {
		case <-cli.ctx.Done():
			return
		case <-time.After(time.Second):
		}
	}
}

func (cli *SyncClient) Stop() { cli.stop() }

// peek top dirty key and data
func (cli *SyncClient) peek() (string, xData, error) {
	return cli.runScript(luaPeek, "", 0)
}

// clean dirty flag and make key volatile, then peek the next
func (cli *SyncClient) next(key string, rev int64) (string, xData, error) {
	return cli.runScript(luaNext, key, rev)
}

func (cli *SyncClient) runScript(script *Script, key string, rev int64) (
	_ string, data xData, err error) {
	var (
		keys []string
		args []interface{}
	)
	if key != "" && rev > 0 {
		keys, args = []string{key}, []interface{}{rev}
	}
	r, err := script.Run(cli.ctx, cli.rdb, keys, args...).Result()
	if err != nil {
		return
	}
	a, ok := r.([]interface{})
	if !ok || len(a) != 2 {
		panic(fmt.Errorf("unexpected return type: %T", r))
	}
	if err = msgpack.Unmarshal(
		fastStringToBytes(a[1].(string)), &data); err != nil {
		return
	}
	return a[0].(string), data, nil
}

func (cli *SyncClient) save(key string, data xData) (err error) {
	database, collection, _id := cli.opts.keyMappingStrategy.MapKey(key)
	_, err = cli.mdb.Database(database).Collection(collection).UpdateOne(
		context.Background(),
		bson.M{"_id": _id},
		bson.M{"$set": &xDataBytes{
			Rev: data.Rev,
			Val: fastStringToBytes(data.Val),
		}},
		options.Update().SetUpsert(true),
	)
	return
}
