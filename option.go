package remon

import (
	"strings"
	"time"
)

// map redis key to mongodb (database,collection,_id)
type KeyMappingStrategy interface {
	MapKey(string) (_, _, _ string)
}

type funcKeyMappingStrategy struct {
	f func(string) (_, _, _ string)
}

func (f funcKeyMappingStrategy) MapKey(key string) (_, _, _ string) {
	return f.f(key)
}

type defaultKeyMappingStrategy struct {
}

func (defaultKeyMappingStrategy) MapKey(key string) (_, _, _ string) {
	a := strings.SplitN(key, ":", 3)
	switch len(a) {
	case 3:
		return a[0], a[1], a[2]
	case 2:
		return "default", a[0], a[1]
	default:
		return "default", "default", key
	}
}

// ReMon/Sync options
type options struct {
	// volatile ttl could be a very long time while redis maxmemory-policy was set to volatile-lru or volatile-lfu
	volatileTTL time.Duration
	// map redis key to mongodb (database,collection,_id)
	keyMappingStrategy KeyMappingStrategy
}

func newOptions() *options {
	return &options{
		volatileTTL:        24 * time.Hour,
		keyMappingStrategy: defaultKeyMappingStrategy{},
	}
}

type Option interface {
	apply(o *options)
}

type funcOption struct {
	f func(o *options)
}

func (f funcOption) apply(o *options) {
	f.f(o)
}

func WithVolatileTTL(v time.Duration) Option {
	return funcOption{func(o *options) {
		o.volatileTTL = v
	}}
}

func WithKeyMappingStrategy(v KeyMappingStrategy) Option {
	return funcOption{func(o *options) {
		o.keyMappingStrategy = v
	}}
}
func WithKeyMappingStrategyFunc(v func(string) (_, _, _ string)) Option {
	return funcOption{func(o *options) {
		o.keyMappingStrategy = funcKeyMappingStrategy{v}
	}}
}

// get options
type getOptions struct {
	addOnNotExist      bool
	addOnNotExistValue string
}

type GetOption interface {
	apply(o *getOptions)
}

type funcGetOption struct {
	f func(o *getOptions)
}

func (f funcGetOption) apply(o *getOptions) {
	f.f(o)
}

func AddOnNotExist(v string) GetOption {
	return funcGetOption{func(o *getOptions) {
		o.addOnNotExist = true
		o.addOnNotExistValue = v
	}}
}

func applyOptions(o *options, opts []Option) {
	for _, opt := range opts {
		opt.apply(o)
	}
}
func applyGetOptions(o *getOptions, opts []GetOption) {
	for _, opt := range opts {
		opt.apply(o)
	}
}
