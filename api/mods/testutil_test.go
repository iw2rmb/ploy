package mods

// kvMem is a lightweight in-memory KV used in tests
type kvMem struct{ m map[string][]byte }

func (k *kvMem) Put(key string, v []byte) error {
	if k.m == nil {
		k.m = map[string][]byte{}
	}
	k.m[key] = append([]byte(nil), v...)
	return nil
}
func (k *kvMem) Get(key string) ([]byte, error) {
	if k.m == nil {
		return nil, nil
	}
	return k.m[key], nil
}
func (k *kvMem) Keys(prefix, sep string) ([]string, error) {
	keys := []string{}
	for k2 := range k.m {
		if len(prefix) == 0 || (len(k2) >= len(prefix) && k2[:len(prefix)] == prefix) {
			keys = append(keys, k2)
		}
	}
	return keys, nil
}
func (k *kvMem) Delete(key string) error { delete(k.m, key); return nil }
