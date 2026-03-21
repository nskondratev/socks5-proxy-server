package main

import (
	"net"
	"os"
	"path/filepath"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestEncodeAddr(t *testing.T) {
	t.Run("ipv4", func(t *testing.T) {
		addrType, payload, err := encodeAddr("127.0.0.1")
		require.NoError(t, err)
		assert.Equal(t, byte(0x01), addrType)
		assert.Len(t, payload, 4)
	})

	t.Run("ipv6", func(t *testing.T) {
		addrType, payload, err := encodeAddr("::1")
		require.NoError(t, err)
		assert.Equal(t, byte(0x04), addrType)
		assert.Len(t, payload, 16)
	})

	t.Run("domain", func(t *testing.T) {
		addrType, payload, err := encodeAddr("example.com")
		require.NoError(t, err)
		assert.Equal(t, byte(0x03), addrType)
		require.NotEmpty(t, payload)
		assert.Equal(t, byte(len("example.com")), payload[0])
		assert.Equal(t, "example.com", string(payload[1:]))
	})

	t.Run("domain too long", func(t *testing.T) {
		host := make([]byte, 256)
		for i := range host {
			host[i] = 'a'
		}

		_, _, err := encodeAddr(string(host))
		require.Error(t, err)
		assert.ErrorContains(t, err, "domain too long")
	})
}

func TestConsumeBindAddr(t *testing.T) {
	t.Run("ipv4", func(t *testing.T) {
		c1, c2 := net.Pipe()
		defer c1.Close()
		defer c2.Close()

		go func() {
			_, _ = c2.Write([]byte{1, 2, 3, 4, 0, 80})
		}()

		require.NoError(t, consumeBindAddr(c1, 0x01))
	})

	t.Run("domain", func(t *testing.T) {
		c1, c2 := net.Pipe()
		defer c1.Close()
		defer c2.Close()

		go func() {
			_, _ = c2.Write([]byte{3, 'a', 'b', 'c', 0, 80})
		}()

		require.NoError(t, consumeBindAddr(c1, 0x03))
	})

	t.Run("ipv6", func(t *testing.T) {
		c1, c2 := net.Pipe()
		defer c1.Close()
		defer c2.Close()

		go func() {
			_, _ = c2.Write(make([]byte, 18))
		}()

		require.NoError(t, consumeBindAddr(c1, 0x04))
	})

	t.Run("unsupported type", func(t *testing.T) {
		c1, c2 := net.Pipe()
		defer c1.Close()
		defer c2.Close()
		require.Error(t, consumeBindAddr(c1, 0x09))
	})
}

func TestPercentile(t *testing.T) {
	sorted := []float64{10, 20, 30, 40}

	assert.Equal(t, float64(0), percentile(nil, 50))
	assert.Equal(t, float64(10), percentile(sorted, 0))
	assert.Equal(t, float64(40), percentile(sorted, 100))
	assert.Equal(t, float64(25), percentile(sorted, 50))
	assert.Equal(t, float64(37), percentile(sorted, 90))
}

func TestUpdateMax(t *testing.T) {
	var current int64
	updateMax(&current, 5)
	updateMax(&current, 3)
	updateMax(&current, 10)
	assert.Equal(t, int64(10), atomic.LoadInt64(&current))
}

func TestCollectResources(t *testing.T) {
	ch := make(chan resourceSample, 2)
	ch <- resourceSample{CPUPercent: 10}
	ch <- resourceSample{CPUPercent: 20}
	close(ch)

	res := collectResources(ch)
	require.Len(t, res, 2)
	assert.Equal(t, float64(10), res[0].CPUPercent)
	assert.Equal(t, float64(20), res[1].CPUPercent)
}

func TestSinkListenAddr(t *testing.T) {
	addr, err := sinkListenAddr("example.com:18080")
	require.NoError(t, err)
	assert.Equal(t, "0.0.0.0:18080", addr)

	_, err = sinkListenAddr("bad")
	require.Error(t, err)
}

func TestRewriteLocalSinkTarget(t *testing.T) {
	t.Run("rewrites container host alias", func(t *testing.T) {
		targetAddr, rewritten, err := rewriteLocalSinkTargetWithIP("host.docker.internal:18080", "127.0.0.1")
		require.NoError(t, err)
		require.True(t, rewritten)
		assert.Equal(t, "127.0.0.1:18080", targetAddr)
	})

	t.Run("keeps explicit remote host", func(t *testing.T) {
		targetAddr, rewritten, err := rewriteLocalSinkTargetWithIP("example.com:18080", "127.0.0.1")
		require.NoError(t, err)
		require.False(t, rewritten)
		assert.Equal(t, "example.com:18080", targetAddr)
	})

	t.Run("rejects invalid target", func(t *testing.T) {
		_, _, err := rewriteLocalSinkTargetWithIP("bad", "127.0.0.1")
		require.Error(t, err)
		assert.ErrorContains(t, err, "split local sink target address")
	})
}

func TestShouldRewriteLocalSinkHost(t *testing.T) {
	assert.True(t, shouldRewriteLocalSinkHost("host.docker.internal"))
	assert.True(t, shouldRewriteLocalSinkHost("localhost"))
	assert.True(t, shouldRewriteLocalSinkHost("127.0.0.1"))
	assert.False(t, shouldRewriteLocalSinkHost("example.com"))
}

func TestBuildReport(t *testing.T) {
	start := time.Now()
	end := start.Add(10 * time.Second)
	cfg := config{TotalConnections: 3}

	samples := []connectionSample{
		{ID: 1, ConnectLatency: 10 * time.Millisecond, SessionDuration: 100 * time.Millisecond, BytesTransferred: 1024},
		{ID: 2, ConnectLatency: 20 * time.Millisecond, SessionDuration: 200 * time.Millisecond, BytesTransferred: 2048},
		{ID: 3, Error: "failed"},
	}

	rep := buildReport(cfg, start, end, 7, samples, []resourceSample{{CPUPercent: 11}})

	assert.Equal(t, 3, rep.RequestedConnections)
	assert.Equal(t, 2, rep.CompletedConnections)
	assert.Equal(t, 1, rep.FailedConnections)
	assert.Equal(t, int64(7), rep.MaxSimultaneousConns)
	assert.NotZero(t, rep.ConnectLatencyP95Ms)
	assert.NotZero(t, rep.ConnectLatencyP99Ms)
	assert.NotZero(t, rep.SessionDurationAvgMs)
	assert.NotZero(t, rep.ThroughputTotalMBSec)
	assert.NotZero(t, rep.ThroughputPerConnMBSec)
	assert.NotZero(t, rep.TotalTransferredMB)
	require.Len(t, rep.ResourceSamples, 1)
}

func TestWriteReports(t *testing.T) {
	dir := t.TempDir()
	rep := report{
		CreatedAt:            time.Now(),
		RequestedConnections: 1,
		CompletedConnections: 1,
		MaxSimultaneousConns: 1,
		Connections: []connectionSample{
			{ID: 1, ConnectLatency: time.Millisecond, SessionDuration: 2 * time.Millisecond, BytesTransferred: 10},
		},
		ResourceSamples: []resourceSample{
			{At: time.Now(), CPUPercent: 5, RSSMB: 10},
		},
	}

	err := writeReports(dir, "ts", rep)
	require.NoError(t, err)

	jsonPath := filepath.Join(dir, "loadtest-ts.json")
	csvPath := filepath.Join(dir, "connections-ts.csv")
	resourcePath := filepath.Join(dir, "resources-ts.csv")
	mdPath := filepath.Join(dir, "summary-ts.md")

	assert.FileExists(t, jsonPath)
	assert.FileExists(t, csvPath)
	assert.FileExists(t, resourcePath)
	assert.FileExists(t, mdPath)

	content, err := os.ReadFile(mdPath)
	require.NoError(t, err)
	assert.Contains(t, string(content), "SOCKS5 load test summary")
}
