package auth

import (
	"sync"
	"testing"
)

func TestMemoryCache_GetSet(t *testing.T) {
	c := newMemoryCache(map[string]string{"a": "1"})

	if v, ok := c.Get("a"); !ok || v != "1" {
		t.Fatalf("Get(a) = %q, %v; want 1, true", v, ok)
	}
	if _, ok := c.Get("missing"); ok {
		t.Fatal("Get(missing) should be false")
	}

	c.Set("b", "2")
	if v, ok := c.Get("b"); !ok || v != "2" {
		t.Fatalf("Get(b) = %q, %v; want 2, true", v, ok)
	}

	c.Set("a", "updated")
	if v, _ := c.Get("a"); v != "updated" {
		t.Fatalf("Get(a) after update = %q; want updated", v)
	}
}

func TestMemoryCache_Concurrent(t *testing.T) {
	c := newMemoryCache(nil)
	var wg sync.WaitGroup

	for i := 0; i < 50; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			key := string(rune('a' + i%26))
			c.Set(key, "v")
			c.Get(key)
		}(i)
	}
	wg.Wait()
}
