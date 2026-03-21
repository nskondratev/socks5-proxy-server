package main

import (
	"bufio"
	"context"
	"encoding/csv"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"math"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"sort"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"
)

const (
	defaultProxyAddr          = "127.0.0.1:8000"
	defaultTargetAddr         = "127.0.0.1:18080"
	defaultConcurrency        = 200
	defaultTotalConnections   = 2000
	defaultPayloadBytes       = 262144
	defaultReadBufferBytes    = 32768
	resourceSampleIntervalSec = 1
)

type config struct {
	ProxyAddr        string        `json:"proxy_addr"`
	TargetAddr       string        `json:"target_addr"`
	Username         string        `json:"username"`
	Password         string        `json:"password"`
	Concurrency      int           `json:"concurrency"`
	TotalConnections int           `json:"total_connections"`
	PayloadBytes     int           `json:"payload_bytes"`
	DialTimeout      time.Duration `json:"dial_timeout"`
	ReadTimeout      time.Duration `json:"read_timeout"`
	ReportDir        string        `json:"report_dir"`
	ProxyPID         int           `json:"proxy_pid"`
	UseLocalSink     bool          `json:"use_local_sink"`
}

type connectionSample struct {
	ID                 int           `json:"id"`
	ConnectLatency     time.Duration `json:"connect_latency"`
	TransferDuration   time.Duration `json:"transfer_duration"`
	SessionDuration    time.Duration `json:"session_duration"`
	BytesTransferred   int64         `json:"bytes_transferred"`
	ThroughputBytesSec float64       `json:"throughput_bytes_sec"`
	Error              string        `json:"error,omitempty"`
}

type resourceSample struct {
	At         time.Time `json:"at"`
	CPUPercent float64   `json:"cpu_percent"`
	RSSMB      float64   `json:"rss_mb"`
	Error      string    `json:"error,omitempty"`
}

type report struct {
	CreatedAt              time.Time          `json:"created_at"`
	Config                 config             `json:"config"`
	RequestedConnections   int                `json:"requested_connections"`
	CompletedConnections   int                `json:"completed_connections"`
	FailedConnections      int                `json:"failed_connections"`
	MaxSimultaneousConns   int64              `json:"max_simultaneous_connections"`
	ConnectLatencyP95Ms    float64            `json:"connect_latency_p95_ms"`
	ConnectLatencyP98Ms    float64            `json:"connect_latency_p98_ms"`
	ConnectLatencyP99Ms    float64            `json:"connect_latency_p99_ms"`
	ConnectLatencyP999Ms   float64            `json:"connect_latency_p999_ms"`
	SessionDurationAvgMs   float64            `json:"session_duration_avg_ms"`
	ThroughputTotalMBSec   float64            `json:"throughput_total_mb_sec"`
	ThroughputPerConnMBSec float64            `json:"throughput_per_connection_mb_sec"`
	TotalTransferredMB     float64            `json:"total_transferred_mb"`
	ResourceSamples        []resourceSample   `json:"resource_samples"`
	Connections            []connectionSample `json:"connections"`
}

func main() {
	if err := run(); err != nil {
		exitErr(err)
	}
}

func run() error {
	cfg, err := parseFlags()
	if err != nil {
		return err
	}

	cfg, err = normalizeLocalSinkTarget(cfg)
	if err != nil {
		return err
	}

	if cfg.UseLocalSink {
		sinkAddr, err := sinkListenAddr(cfg.TargetAddr)
		if err != nil {
			return fmt.Errorf("failed to resolve local sink bind address from target %q: %w", cfg.TargetAddr, err)
		}

		stopSink, err := runLocalSink(sinkAddr)
		if err != nil {
			return fmt.Errorf("failed to run local sink server on %s: %w", sinkAddr, err)
		}
		defer stopSink()
	}

	if err := os.MkdirAll(cfg.ReportDir, 0o755); err != nil {
		return fmt.Errorf("failed to create report dir: %w", err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	resourceCh := make(chan resourceSample, 256)
	var wg sync.WaitGroup

	if cfg.ProxyPID > 0 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			sampleProxyResources(ctx, cfg.ProxyPID, resourceCh)
		}()
	}

	startedAt := time.Now()
	samples, maxActive, runErr := runLoad(cfg)
	finishedAt := time.Now()

	cancel()
	wg.Wait()
	close(resourceCh)

	resources := collectResources(resourceCh)

	rep := buildReport(cfg, startedAt, finishedAt, maxActive, samples, resources)

	timestamp := time.Now().Format("20060102-150405")
	if err := writeReports(cfg.ReportDir, timestamp, rep); err != nil {
		return fmt.Errorf("failed to write reports: %w", err)
	}

	printRunSummary(cfg.ReportDir, rep)

	if runErr != nil {
		return runErr
	}

	return nil
}

func printRunSummary(reportDir string, rep report) {
	fmt.Fprintf(os.Stdout, "Load test finished. Reports written to %s\n", reportDir)
	fmt.Fprintf(
		os.Stdout,
		"Completed: %d, Failed: %d, Max simultaneous: %d\n",
		rep.CompletedConnections,
		rep.FailedConnections,
		rep.MaxSimultaneousConns,
	)
	fmt.Fprintf(
		os.Stdout,
		"Connect latency p95=%.2fms p98=%.2fms p99=%.2fms p99.9=%.2fms\n",
		rep.ConnectLatencyP95Ms,
		rep.ConnectLatencyP98Ms,
		rep.ConnectLatencyP99Ms,
		rep.ConnectLatencyP999Ms,
	)
	fmt.Fprintf(
		os.Stdout,
		"Throughput total=%.2f MB/s, per connection=%.2f MB/s\n",
		rep.ThroughputTotalMBSec,
		rep.ThroughputPerConnMBSec,
	)
}

func parseFlags() (config, error) {
	cfg := config{}
	flag.StringVar(&cfg.ProxyAddr, "proxy-addr", defaultProxyAddr, "SOCKS5 proxy address host:port")
	flag.StringVar(&cfg.TargetAddr, "target-addr", defaultTargetAddr, "Target host:port requested through the proxy")
	flag.StringVar(&cfg.Username, "username", "", "SOCKS5 username (optional)")
	flag.StringVar(&cfg.Password, "password", "", "SOCKS5 password (optional)")
	flag.IntVar(&cfg.Concurrency, "concurrency", defaultConcurrency, "Number of concurrent connections")
	flag.IntVar(&cfg.TotalConnections, "total-connections", defaultTotalConnections, "Total number of connections to execute")
	flag.IntVar(&cfg.PayloadBytes, "payload-bytes", defaultPayloadBytes, "Bytes to download per connection")
	flag.DurationVar(&cfg.DialTimeout, "dial-timeout", 10*time.Second, "Timeout per connection establishment")
	flag.DurationVar(&cfg.ReadTimeout, "read-timeout", 30*time.Second, "Timeout per connection transfer")
	flag.StringVar(&cfg.ReportDir, "report-dir", "reports/loadtest", "Output directory for generated reports")
	flag.IntVar(&cfg.ProxyPID, "proxy-pid", 0, "PID of proxy process to sample CPU and RAM")
	flag.BoolVar(&cfg.UseLocalSink, "use-local-sink", true, "Start local sink server for deterministic traffic target")
	flag.Parse()

	if cfg.Concurrency <= 0 {
		return cfg, errors.New("concurrency must be > 0")
	}
	if cfg.TotalConnections <= 0 {
		return cfg, errors.New("total-connections must be > 0")
	}
	if cfg.PayloadBytes <= 0 {
		return cfg, errors.New("payload-bytes must be > 0")
	}
	if cfg.Username == "" && cfg.Password != "" {
		return cfg, errors.New("password provided without username")
	}

	return cfg, nil
}

func normalizeLocalSinkTarget(cfg config) (config, error) {
	if !cfg.UseLocalSink {
		return cfg, nil
	}

	targetAddr, rewritten, err := rewriteLocalSinkTarget(cfg.TargetAddr)
	if err != nil {
		return cfg, err
	}
	if !rewritten {
		return cfg, nil
	}

	cfg.TargetAddr = targetAddr
	return cfg, nil
}

func rewriteLocalSinkTarget(targetAddr string) (string, bool, error) {
	host, _, err := net.SplitHostPort(targetAddr)
	if err != nil {
		return "", false, fmt.Errorf("split local sink target address: %w", err)
	}
	if !shouldRewriteLocalSinkHost(host) {
		return targetAddr, false, nil
	}

	hostIP, err := detectLocalIPv4()
	if err != nil {
		return "", false, fmt.Errorf("detect local sink host IP: %w", err)
	}

	return rewriteLocalSinkTargetWithIP(targetAddr, hostIP)
}

func rewriteLocalSinkTargetWithIP(targetAddr, hostIP string) (string, bool, error) {
	host, port, err := net.SplitHostPort(targetAddr)
	if err != nil {
		return "", false, fmt.Errorf("split local sink target address: %w", err)
	}
	if !shouldRewriteLocalSinkHost(host) {
		return targetAddr, false, nil
	}

	return net.JoinHostPort(hostIP, port), true, nil
}

func shouldRewriteLocalSinkHost(host string) bool {
	switch host {
	case "host.docker.internal", "localhost", "127.0.0.1", "0.0.0.0", "::1":
		return true
	default:
		return false
	}
}

func detectLocalIPv4() (string, error) {
	if ip, ok := detectDialLocalIPv4(); ok {
		return ip, nil
	}

	if ip, ok := detectInterfaceLocalIPv4(); ok {
		return ip, nil
	}

	return "", errors.New("no active non-loopback IPv4 address found")
}

func detectDialLocalIPv4() (string, bool) {
	conn, err := net.Dial("udp4", "192.0.2.1:80")
	if err != nil {
		return "", false
	}
	defer conn.Close()

	localAddr, ok := conn.LocalAddr().(*net.UDPAddr)
	if !ok {
		return "", false
	}

	ip := localAddr.IP.To4()
	if ip == nil || ip.IsLoopback() {
		return "", false
	}

	return ip.String(), true
}

func detectInterfaceLocalIPv4() (string, bool) {
	ifaces, err := net.Interfaces()
	if err != nil {
		return "", false
	}

	for _, iface := range ifaces {
		if iface.Flags&net.FlagUp == 0 || iface.Flags&net.FlagLoopback != 0 {
			continue
		}

		addrs, err := iface.Addrs()
		if err != nil {
			continue
		}

		for _, addr := range addrs {
			ipNet, ok := addr.(*net.IPNet)
			if !ok {
				continue
			}

			ip := ipNet.IP.To4()
			if ip == nil || ip.IsLoopback() {
				continue
			}

			return ip.String(), true
		}
	}

	return "", false
}

func runLoad(cfg config) ([]connectionSample, int64, error) {
	jobs := make(chan int)
	results := make(chan connectionSample, cfg.TotalConnections)

	var active int64
	var maxActive int64

	var wg sync.WaitGroup
	for workerID := 0; workerID < cfg.Concurrency; workerID++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for id := range jobs {
				current := atomic.AddInt64(&active, 1)
				updateMax(&maxActive, current)
				sample := exerciseConnection(id, cfg)
				results <- sample
				atomic.AddInt64(&active, -1)
			}
		}()
	}

	for i := 0; i < cfg.TotalConnections; i++ {
		jobs <- i
	}
	close(jobs)

	wg.Wait()
	close(results)

	samples := make([]connectionSample, 0, cfg.TotalConnections)
	var failed int
	for sample := range results {
		samples = append(samples, sample)
		if sample.Error != "" {
			failed++
		}
	}

	if failed == len(samples) {
		return samples, maxActive, errors.New("all connections failed")
	}

	return samples, maxActive, nil
}

func exerciseConnection(id int, cfg config) connectionSample {
	sample := connectionSample{ID: id}

	dialStarted := time.Now()
	conn, err := dialSocks5(cfg)
	if err != nil {
		sample.ConnectLatency = time.Since(dialStarted)
		sample.Error = fmt.Sprintf("dial target: %v", err)
		return sample
	}
	defer conn.Close()

	sample.ConnectLatency = time.Since(dialStarted)

	if err := conn.SetDeadline(time.Now().Add(cfg.ReadTimeout)); err != nil {
		sample.Error = fmt.Sprintf("set deadline: %v", err)
		return sample
	}

	transferStarted := time.Now()
	readBuf := make([]byte, defaultReadBufferBytes)
	remaining := cfg.PayloadBytes
	var totalRead int64
	for remaining > 0 {
		chunkSize := len(readBuf)
		if remaining < chunkSize {
			chunkSize = remaining
		}

		n, readErr := conn.Read(readBuf[:chunkSize])
		if n > 0 {
			totalRead += int64(n)
			remaining -= n
		}

		if readErr != nil {
			sample.Error = fmt.Sprintf("read from target: %v", readErr)
			sample.BytesTransferred = totalRead
			sample.TransferDuration = time.Since(transferStarted)
			sample.SessionDuration = sample.ConnectLatency + sample.TransferDuration
			if sample.TransferDuration > 0 {
				sample.ThroughputBytesSec = float64(totalRead) / sample.TransferDuration.Seconds()
			}
			return sample
		}
	}

	sample.BytesTransferred = totalRead
	sample.TransferDuration = time.Since(transferStarted)
	sample.SessionDuration = sample.ConnectLatency + sample.TransferDuration
	if sample.TransferDuration > 0 {
		sample.ThroughputBytesSec = float64(totalRead) / sample.TransferDuration.Seconds()
	}

	return sample
}

func dialSocks5(cfg config) (net.Conn, error) {
	conn, err := net.DialTimeout("tcp", cfg.ProxyAddr, cfg.DialTimeout)
	if err != nil {
		return nil, err
	}

	if err := conn.SetDeadline(time.Now().Add(cfg.DialTimeout)); err != nil {
		_ = conn.Close()
		return nil, err
	}

	if err := socks5Greeting(conn, cfg); err != nil {
		_ = conn.Close()
		return nil, err
	}

	if err := socks5Connect(conn, cfg.TargetAddr); err != nil {
		_ = conn.Close()
		return nil, err
	}

	if err := conn.SetDeadline(time.Time{}); err != nil {
		_ = conn.Close()
		return nil, err
	}

	return conn, nil
}

func socks5Greeting(conn net.Conn, cfg config) error {
	method := byte(0x00)
	methods := []byte{0x00}
	if cfg.Username != "" {
		method = 0x02
		methods = []byte{0x00, 0x02}
	}

	msg := []byte{0x05, byte(len(methods))}
	msg = append(msg, methods...)

	if _, err := conn.Write(msg); err != nil {
		return err
	}

	resp := make([]byte, 2)
	if _, err := io.ReadFull(conn, resp); err != nil {
		return err
	}

	if resp[0] != 0x05 {
		return fmt.Errorf("unexpected socks version in greeting: %d", resp[0])
	}
	if resp[1] == 0xFF {
		return errors.New("proxy rejected authentication methods")
	}
	if resp[1] != method {
		return fmt.Errorf("unexpected auth method selected by proxy: %d", resp[1])
	}

	if method == 0x02 {
		if err := socks5UserPassAuth(conn, cfg.Username, cfg.Password); err != nil {
			return err
		}
	}

	return nil
}

func socks5UserPassAuth(conn net.Conn, username, password string) error {
	if len(username) > 255 || len(password) > 255 {
		return errors.New("username/password too long for socks5")
	}

	req := []byte{0x01, byte(len(username))}
	req = append(req, []byte(username)...)
	req = append(req, byte(len(password)))
	req = append(req, []byte(password)...)

	if _, err := conn.Write(req); err != nil {
		return err
	}

	resp := make([]byte, 2)
	if _, err := io.ReadFull(conn, resp); err != nil {
		return err
	}

	if resp[1] != 0x00 {
		return errors.New("username/password auth failed")
	}

	return nil
}

//nolint:cyclop // SOCKS5 handshake branches are protocol-driven.
func socks5Connect(conn net.Conn, targetAddr string) error {
	host, portRaw, err := net.SplitHostPort(targetAddr)
	if err != nil {
		return fmt.Errorf("invalid target address: %w", err)
	}

	port, err := strconv.Atoi(portRaw)
	if err != nil {
		return fmt.Errorf("invalid target port: %w", err)
	}
	if port <= 0 || port > 65535 {
		return fmt.Errorf("port out of range: %d", port)
	}

	addrType, addrPayload, err := encodeAddr(host)
	if err != nil {
		return err
	}

	req := []byte{0x05, 0x01, 0x00, addrType}
	req = append(req, addrPayload...)
	req = append(req, byte(port>>8), byte(port&0xFF))

	if _, err := conn.Write(req); err != nil {
		return err
	}

	header := make([]byte, 4)
	if _, err := io.ReadFull(conn, header); err != nil {
		return err
	}

	if header[0] != 0x05 {
		return fmt.Errorf("unexpected socks version in connect reply: %d", header[0])
	}
	if header[1] != 0x00 {
		return fmt.Errorf("proxy connect failed, code: 0x%x", header[1])
	}

	if err := consumeBindAddr(conn, header[3]); err != nil {
		return err
	}

	return nil
}

func encodeAddr(host string) (byte, []byte, error) {
	if ip := net.ParseIP(host); ip != nil {
		if ip4 := ip.To4(); ip4 != nil {
			return 0x01, ip4, nil
		}
		ip16 := ip.To16()
		if ip16 == nil {
			return 0, nil, fmt.Errorf("invalid IPv6 host: %s", host)
		}
		return 0x04, ip16, nil
	}

	if len(host) > 255 {
		return 0, nil, fmt.Errorf("domain too long: %s", host)
	}

	payload := []byte{byte(len(host))}
	payload = append(payload, []byte(host)...)
	return 0x03, payload, nil
}

func consumeBindAddr(conn net.Conn, addrType byte) error {
	var toRead int
	switch addrType {
	case 0x01:
		toRead = 4 + 2
	case 0x03:
		length := make([]byte, 1)
		if _, err := io.ReadFull(conn, length); err != nil {
			return err
		}
		toRead = int(length[0]) + 2
	case 0x04:
		toRead = 16 + 2
	default:
		return fmt.Errorf("unsupported addr type in reply: 0x%x", addrType)
	}

	buf := make([]byte, toRead)
	_, err := io.ReadFull(conn, buf)
	return err
}

func buildReport(cfg config, startedAt, finishedAt time.Time, maxActive int64, samples []connectionSample, resources []resourceSample) report {
	rep := report{
		CreatedAt:            time.Now(),
		Config:               cfg,
		RequestedConnections: len(samples),
		MaxSimultaneousConns: maxActive,
		Connections:          samples,
		ResourceSamples:      resources,
	}

	connectLatencies := make([]float64, 0, len(samples))
	var totalDurationSec float64
	var totalSessionMs float64
	var totalBytes int64
	for _, sample := range samples {
		if sample.Error != "" {
			rep.FailedConnections++
			continue
		}

		rep.CompletedConnections++
		connectLatencies = append(connectLatencies, float64(sample.ConnectLatency.Microseconds())/1000)
		totalSessionMs += float64(sample.SessionDuration.Microseconds()) / 1000
		totalBytes += sample.BytesTransferred
	}

	elapsed := finishedAt.Sub(startedAt)
	if elapsed > 0 {
		totalDurationSec = elapsed.Seconds()
	}

	if len(connectLatencies) > 0 {
		sort.Float64s(connectLatencies)
		rep.ConnectLatencyP95Ms = percentile(connectLatencies, 95)
		rep.ConnectLatencyP98Ms = percentile(connectLatencies, 98)
		rep.ConnectLatencyP99Ms = percentile(connectLatencies, 99)
		rep.ConnectLatencyP999Ms = percentile(connectLatencies, 99.9)
	}

	if rep.CompletedConnections > 0 {
		rep.SessionDurationAvgMs = totalSessionMs / float64(rep.CompletedConnections)
		avgSessionSec := rep.SessionDurationAvgMs / 1000
		if avgSessionSec > 0 {
			rep.ThroughputPerConnMBSec = (float64(totalBytes) / float64(rep.CompletedConnections)) / 1024 / 1024 / avgSessionSec
		}
	}

	rep.TotalTransferredMB = float64(totalBytes) / 1024 / 1024
	if totalDurationSec > 0 {
		rep.ThroughputTotalMBSec = rep.TotalTransferredMB / totalDurationSec
	}

	return rep
}

func percentile(sorted []float64, p float64) float64 {
	if len(sorted) == 0 {
		return 0
	}
	if p <= 0 {
		return sorted[0]
	}
	if p >= 100 {
		return sorted[len(sorted)-1]
	}

	rank := p / 100 * float64(len(sorted)-1)
	low := int(math.Floor(rank))
	high := int(math.Ceil(rank))
	if low == high {
		return sorted[low]
	}

	weight := rank - float64(low)
	return sorted[low] + (sorted[high]-sorted[low])*weight
}

func updateMax(current *int64, value int64) {
	for {
		prev := atomic.LoadInt64(current)
		if value <= prev {
			return
		}
		if atomic.CompareAndSwapInt64(current, prev, value) {
			return
		}
	}
}

func collectResources(ch <-chan resourceSample) []resourceSample {
	res := make([]resourceSample, 0)
	for sample := range ch {
		res = append(res, sample)
	}
	return res
}

func sampleProxyResources(ctx context.Context, pid int, out chan<- resourceSample) {
	ticker := time.NewTicker(resourceSampleIntervalSec * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			out <- readProcessStats(pid)
		}
	}
}

func readProcessStats(pid int) resourceSample {
	sample := resourceSample{At: time.Now()}

	var cmd *exec.Cmd
	if runtime.GOOS == "darwin" || runtime.GOOS == "linux" {
		cmd = exec.Command("ps", "-p", strconv.Itoa(pid), "-o", "%cpu=", "-o", "rss=")
	} else {
		sample.Error = "resource sampling unsupported on this OS"
		return sample
	}

	output, err := cmd.Output()
	if err != nil {
		sample.Error = fmt.Sprintf("ps command failed: %v", err)
		return sample
	}

	fields := strings.Fields(string(output))
	if len(fields) < 2 {
		sample.Error = fmt.Sprintf("unexpected ps output: %q", strings.TrimSpace(string(output)))
		return sample
	}

	cpuPercent, cpuErr := strconv.ParseFloat(fields[0], 64)
	rssKB, rssErr := strconv.ParseFloat(fields[1], 64)
	if cpuErr != nil || rssErr != nil {
		sample.Error = fmt.Sprintf("failed to parse ps output: %q", strings.TrimSpace(string(output)))
		return sample
	}

	sample.CPUPercent = cpuPercent
	sample.RSSMB = rssKB / 1024
	return sample
}

func runLocalSink(addr string) (func(), error) {
	listener, err := net.Listen("tcp", addr)
	if err != nil {
		return nil, err
	}

	ctx, cancel := context.WithCancel(context.Background())
	go func() {
		<-ctx.Done()
		_ = listener.Close()
	}()

	go func() {
		for {
			conn, err := listener.Accept()
			if err != nil {
				if errors.Is(err, net.ErrClosed) {
					return
				}
				continue
			}
			go serveSink(conn)
		}
	}()

	return cancel, nil
}

func sinkListenAddr(targetAddr string) (string, error) {
	_, port, err := net.SplitHostPort(targetAddr)
	if err != nil {
		return "", err
	}

	return net.JoinHostPort("0.0.0.0", port), nil
}

func serveSink(conn net.Conn) {
	defer conn.Close()

	payload := make([]byte, 64*1024)
	for i := range payload {
		payload[i] = byte('a' + (i % 26))
	}

	for {
		if _, err := conn.Write(payload); err != nil {
			return
		}
	}
}

func writeReports(reportDir, timestamp string, rep report) error {
	jsonPath := filepath.Join(reportDir, fmt.Sprintf("loadtest-%s.json", timestamp))
	csvPath := filepath.Join(reportDir, fmt.Sprintf("connections-%s.csv", timestamp))
	resourcePath := filepath.Join(reportDir, fmt.Sprintf("resources-%s.csv", timestamp))
	mdPath := filepath.Join(reportDir, fmt.Sprintf("summary-%s.md", timestamp))

	if err := writeJSON(jsonPath, rep); err != nil {
		return err
	}
	if err := writeConnectionsCSV(csvPath, rep.Connections); err != nil {
		return err
	}
	if err := writeResourcesCSV(resourcePath, rep.ResourceSamples); err != nil {
		return err
	}
	if err := writeSummaryMarkdown(mdPath, jsonPath, csvPath, resourcePath, rep); err != nil {
		return err
	}

	return nil
}

func writeJSON(path string, rep report) error {
	file, err := os.Create(path)
	if err != nil {
		return err
	}
	defer file.Close()

	enc := json.NewEncoder(file)
	enc.SetIndent("", "  ")
	return enc.Encode(rep)
}

func writeConnectionsCSV(path string, samples []connectionSample) error {
	file, err := os.Create(path)
	if err != nil {
		return err
	}
	defer file.Close()

	writer := csv.NewWriter(file)
	defer writer.Flush()

	header := []string{"id", "connect_latency_ms", "transfer_duration_ms", "session_duration_ms", "bytes_transferred", "throughput_bytes_sec", "error"}
	if err := writer.Write(header); err != nil {
		return err
	}

	for _, sample := range samples {
		record := []string{
			strconv.Itoa(sample.ID),
			fmt.Sprintf("%.3f", float64(sample.ConnectLatency.Microseconds())/1000),
			fmt.Sprintf("%.3f", float64(sample.TransferDuration.Microseconds())/1000),
			fmt.Sprintf("%.3f", float64(sample.SessionDuration.Microseconds())/1000),
			strconv.FormatInt(sample.BytesTransferred, 10),
			fmt.Sprintf("%.3f", sample.ThroughputBytesSec),
			sample.Error,
		}
		if err := writer.Write(record); err != nil {
			return err
		}
	}

	return writer.Error()
}

func writeResourcesCSV(path string, samples []resourceSample) error {
	file, err := os.Create(path)
	if err != nil {
		return err
	}
	defer file.Close()

	writer := csv.NewWriter(file)
	defer writer.Flush()

	header := []string{"timestamp", "cpu_percent", "rss_mb", "error"}
	if err := writer.Write(header); err != nil {
		return err
	}

	for _, sample := range samples {
		record := []string{
			sample.At.Format(time.RFC3339Nano),
			fmt.Sprintf("%.3f", sample.CPUPercent),
			fmt.Sprintf("%.3f", sample.RSSMB),
			sample.Error,
		}
		if err := writer.Write(record); err != nil {
			return err
		}
	}

	return writer.Error()
}

func writeSummaryMarkdown(path, jsonPath, csvPath, resourcePath string, rep report) error {
	file, err := os.Create(path)
	if err != nil {
		return err
	}
	defer file.Close()

	w := bufio.NewWriter(file)
	defer w.Flush()

	_, err = fmt.Fprintf(
		w,
		"# SOCKS5 load test summary\n\nGenerated: %s\n\n## Core metrics\n- Requested connections: %d\n- Completed connections: %d\n- Failed connections: %d\n- Max simultaneous connections: %d\n- Connect latency p95/p98/p99/p99.9 (ms): %.3f / %.3f / %.3f / %.3f\n- Average connection time (ms): %.3f\n- Throughput total (MB/s): %.3f\n- Throughput per connection (MB/s): %.3f\n- Total transferred (MB): %.3f\n\n## Reports\n- JSON: `%s`\n- Connection CSV: `%s`\n- Resource CSV: `%s`\n",
		rep.CreatedAt.Format(time.RFC3339),
		rep.RequestedConnections,
		rep.CompletedConnections,
		rep.FailedConnections,
		rep.MaxSimultaneousConns,
		rep.ConnectLatencyP95Ms,
		rep.ConnectLatencyP98Ms,
		rep.ConnectLatencyP99Ms,
		rep.ConnectLatencyP999Ms,
		rep.SessionDurationAvgMs,
		rep.ThroughputTotalMBSec,
		rep.ThroughputPerConnMBSec,
		rep.TotalTransferredMB,
		jsonPath,
		csvPath,
		resourcePath,
	)
	if err != nil {
		return err
	}

	return nil
}

func exitErr(err error) {
	fmt.Fprintf(os.Stderr, "error: %v\n", err)
	os.Exit(1)
}
