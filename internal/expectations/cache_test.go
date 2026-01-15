package expectations_test

import (
	"strconv"
	"sync"
	"testing"

	"github.com/lablabs/pod-deletion-cost-controller/internal/expectations"
	"github.com/stretchr/testify/require"
)

func TestCacheBasicOperations(t *testing.T) {
	cache := expectations.NewCache[string, int]()

	// Test Set and Get
	cache.Set("a", 1)
	val, ok := cache.Get("a")
	require.True(t, ok)
	require.Equal(t, 1, val)

	// Test Has
	require.True(t, cache.Has("a"))
	require.False(t, cache.Has("b"))

	// Test GetList
	cache.Set("b", 2)
	cache.Set("c", 3)
	list := cache.GetList("a", "b", "x")
	require.ElementsMatch(t, []int{1, 2}, list) // "x" is missing → ignored

	// Test Delete
	cache.Delete("a")
	_, ok = cache.Get("a")
	require.False(t, ok)
	require.False(t, cache.Has("a"))
}

func TestCacheConcurrentAccess(t *testing.T) {
	cache := expectations.NewCache[int, string]()
	wg := sync.WaitGroup{}
	n := 1000

	// Concurrent Set
	for i := 0; i < n; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			cache.Set(i, "val"+strconv.Itoa(i))
		}(i)
	}

	// Concurrent Get
	for i := 0; i < n; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			cache.Get(i)
		}(i)
	}

	wg.Wait()

	// Verify
	for i := 0; i < n; i++ {
		val, ok := cache.Get(i)
		require.True(t, ok)
		require.Equal(t, "val"+strconv.Itoa(i), val)
	}
}

func TestCacheGetListEmptyAndPartial(t *testing.T) {
	cache := expectations.NewCache[string, int]()
	cache.Set("x", 10)
	cache.Set("y", 20)

	tests := []struct {
		name     string
		keys     []string
		expected []int
	}{
		{"all present", []string{"x", "y"}, []int{10, 20}},
		{"some missing", []string{"x", "z"}, []int{10}},
		{"all missing", []string{"a", "b"}, []int{}},
		{"empty keys", []string{}, []int{}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			out := cache.GetList(tt.keys...)
			require.ElementsMatch(t, tt.expected, out)
		})
	}
}
