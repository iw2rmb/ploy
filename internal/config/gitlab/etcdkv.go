package gitlab

import (
	"context"

	clientv3 "go.etcd.io/etcd/client/v3"
)

// etcdKV adapts a clientv3.KV to the local KV interface.
type etcdKV struct {
	client clientv3.KV
}

// NewEtcdKV wraps an etcd KV client for use with the GitLab Store.
func NewEtcdKV(client clientv3.KV) KV {
	return &etcdKV{client: client}
}

func (e *etcdKV) Get(ctx context.Context, key string) (Value, bool, error) {
	resp, err := e.client.Get(ctx, key)
	if err != nil {
		return Value{}, false, err
	}
	if len(resp.Kvs) == 0 {
		return Value{}, false, nil
	}
	kv := resp.Kvs[0]
	return Value{Data: string(kv.Value), Revision: kv.ModRevision}, true, nil
}

func (e *etcdKV) Put(ctx context.Context, key, value string) (int64, error) {
	resp, err := e.client.Put(ctx, key, value)
	if err != nil {
		return 0, err
	}
	if resp == nil || resp.Header == nil {
		return 0, nil
	}
	return resp.Header.Revision, nil
}
