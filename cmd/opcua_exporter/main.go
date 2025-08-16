package main

import (
	"context"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"github.com/gopcua/opcua"
	opcua_debug "github.com/gopcua/opcua/debug"
	"github.com/gopcua/opcua/monitor"
	"github.com/spf13/pflag"
	"github.com/spf13/viper"

	"github.com/mwieczorkiewicz/opcua_exporter/internal/config"
	"github.com/mwieczorkiewicz/opcua_exporter/internal/connection"
	"github.com/mwieczorkiewicz/opcua_exporter/internal/handlers"
	"github.com/mwieczorkiewicz/opcua_exporter/internal/metrics"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promhttp"
)

const (
	timeNode           string = "i=2258"            // Server time node ID
	timeNodeMetricName string = "opcua_server_time" // Metric name for server time

	// Configuration flag names
	flagPort                = "port"
	flagEndpoint            = "endpoint"
	flagPromPrefix          = "prom-prefix"
	flagDebug               = "debug"
	flagReadTimeout         = "read-timeout"
	flagMaxTimeouts         = "max-timeouts"
	flagBufferSize          = "buffer-size"
	flagSummaryInterval     = "summary-interval"
	flagSubscribeToTimeNode = "subscribe-to-time-node"
	flagNode                = "node"
	flagConfig              = "config"

	// Default values
	defaultPort            = 9686
	defaultEndpoint        = "opc.tcp://localhost:4096"
	defaultReadTimeout     = 5 * time.Second
	defaultMaxTimeouts     = 0
	defaultBufferSize      = 64
	defaultSummaryInterval = 5 * time.Minute

	// HTTP server timeouts
	httpReadTimeout  = 15 * time.Second
	httpWriteTimeout = 15 * time.Second
	httpIdleTimeout  = 60 * time.Second
	shutdownTimeout  = 5 * time.Second

	// Other constants
	subsystemName          = "opcua_exporter"
	uptimeMetricName       = "uptime_seconds"
	messageCountMetricName = "message_count"
	metricsPath            = "/metrics"
)


var startTime = time.Now()
var uptimeGauge prometheus.Gauge
var messageCounter prometheus.Counter
var eventSummaryCounter *handlers.EventSummaryCounter

// App holds the application state and context
type App struct {
	ctx    context.Context
	cancel context.CancelFunc
	client *opcua.Client
	server *http.Server
}

// shutdown performs graceful shutdown of the application
func (app *App) shutdown(err error) {
	log.Printf("Shutting down: %v", err)
	if app.client != nil {
		app.client.Close(app.ctx)
	}
	if app.server != nil {
		ctx, cancel := context.WithTimeout(context.Background(), shutdownTimeout)
		defer cancel()
		app.server.Shutdown(ctx)
	}
	app.cancel()
	os.Exit(1)
}

func init() {
	uptimeGauge = prometheus.NewGauge(prometheus.GaugeOpts{
		Subsystem: subsystemName,
		Name:      uptimeMetricName,
		Help:      "Time in seconds since the OPCUA exporter started",
	})
	uptimeGauge.Set(time.Since(startTime).Seconds())
	prometheus.MustRegister(uptimeGauge)

	messageCounter = prometheus.NewCounter(prometheus.CounterOpts{
		Subsystem: subsystemName,
		Name:      messageCountMetricName,
		Help:      "Total number of OPCUA channel updates received by the exporter",
	})
	prometheus.MustRegister(messageCounter)
}




// parseNodeFlag parses a node flag string in format "nodeId,metricName[,extractBit]"
func parseNodeFlag(nodeFlag string) (config.NodeMapping, error) {
	parts := strings.Split(nodeFlag, ",")
	if len(parts) < 2 {
		return config.NodeMapping{}, fmt.Errorf("expected format 'nodeId,metricName[,extractBit]'")
	}

	nodeMapping := config.NodeMapping{
		NodeName:   strings.TrimSpace(parts[0]),
		MetricName: strings.TrimSpace(parts[1]),
	}

	if len(parts) >= 3 {
		extractBitStr := strings.TrimSpace(parts[2])
		if extractBitStr != "" {
			var extractBit int
			if _, err := fmt.Sscanf(extractBitStr, "%d", &extractBit); err != nil {
				return config.NodeMapping{}, fmt.Errorf("invalid extractBit value '%s'", extractBitStr)
			}
			nodeMapping.ExtractBit = extractBit
		}
	}

	return nodeMapping, nil
}

func parseFlags() (string, error) {
	var configFile string
	pflag.StringVar(&configFile, flagConfig, "", "Path to YAML configuration file")
	pflag.Int(flagPort, defaultPort, "Port to publish metrics on")
	pflag.String(flagEndpoint, defaultEndpoint, "OPC UA Endpoint to connect to")
	pflag.String(flagPromPrefix, "", "Prefix for prometheus metrics")
	pflag.Bool(flagDebug, false, "Enable debug logging")
	pflag.Duration(flagReadTimeout, defaultReadTimeout, "Timeout for OPCUA subscription messages")
	pflag.Int(flagMaxTimeouts, defaultMaxTimeouts, "Max timeouts before quitting (0=disabled)")
	pflag.Int(flagBufferSize, defaultBufferSize, "Message buffer size")
	pflag.Duration(flagSummaryInterval, defaultSummaryInterval, "Event count summary interval")
	pflag.Bool(flagSubscribeToTimeNode, false, "Subscribe to server time node")
	pflag.StringArray(flagNode, []string{}, "Node mapping: 'nodeId,metricName[,extractBit]'")
	pflag.Parse()
	return configFile, nil
}

func loadAndApplyConfig(configFile string) (*config.Config, error) {
	cfg, err := config.Load(configFile)
	if err != nil {
		return nil, fmt.Errorf("error loading configuration: %w", err)
	}

	// Bind command-line flags to override configuration
	viper.BindPFlags(pflag.CommandLine)

	// Apply flag overrides to configuration
	if viper.IsSet(flagPort) {
		cfg.Port = viper.GetInt(flagPort)
	}
	if viper.IsSet(flagEndpoint) {
		cfg.Endpoint = viper.GetString(flagEndpoint)
	}
	if viper.IsSet(flagPromPrefix) {
		cfg.PromPrefix = viper.GetString(flagPromPrefix)
	}
	if viper.IsSet(flagDebug) {
		cfg.Debug = viper.GetBool(flagDebug)
	}
	if viper.IsSet(flagReadTimeout) {
		cfg.ReadTimeout = viper.GetDuration(flagReadTimeout)
	}
	if viper.IsSet(flagMaxTimeouts) {
		cfg.MaxTimeouts = viper.GetInt(flagMaxTimeouts)
	}
	if viper.IsSet(flagBufferSize) {
		cfg.BufferSize = viper.GetInt(flagBufferSize)
	}
	if viper.IsSet(flagSummaryInterval) {
		cfg.SummaryInterval = viper.GetDuration(flagSummaryInterval)
	}
	if viper.IsSet(flagSubscribeToTimeNode) {
		cfg.SubscribeToTimeNode = viper.GetBool(flagSubscribeToTimeNode)
	}

	// Parse node flags and add to configuration
	nodeFlags := viper.GetStringSlice(flagNode)
	for _, nodeFlag := range nodeFlags {
		nodeMapping, err := parseNodeFlag(nodeFlag)
		if err != nil {
			log.Printf("Invalid node flag '%s': %v", nodeFlag, err)
			continue
		}
		cfg.AddNodeMapping(nodeMapping)
		log.Printf("Added node mapping from flag: %s -> %s", nodeMapping.NodeName, nodeMapping.MetricName)
	}

	return cfg, nil
}

func setupOPCUAClient(ctx context.Context, cfg *config.Config) (*opcua.Client, error) {
	connManager := connection.NewManager(cfg.Endpoint)
	return connManager.Connect(ctx)
}

func createHTTPServer(port int) *http.Server {
	http.Handle(metricsPath, promhttp.Handler())
	listenOn := fmt.Sprintf(":%d", port)

	return &http.Server{
		Addr:         listenOn,
		Handler:      nil,
		ReadTimeout:  httpReadTimeout,
		WriteTimeout: httpWriteTimeout,
		IdleTimeout:  httpIdleTimeout,
	}
}

func main() {
	log.Print("Starting up.")

	configFile, err := parseFlags()
	if err != nil {
		log.Fatalf("Error parsing flags: %v", err)
	}

	cfg, err := loadAndApplyConfig(configFile)
	if err != nil {
		log.Fatalf("%v", err)
	}

	opcua_debug.Enable = cfg.Debug

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	app := &App{
		ctx:    ctx,
		cancel: cancel,
	}

	// Handle graceful shutdown on signals
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, os.Interrupt, syscall.SIGTERM)
	go func() {
		<-sigChan
		log.Print("Received shutdown signal")
		app.shutdown(nil)
	}()

	eventSummaryCounter = handlers.NewEventSummaryCounter(cfg.SummaryInterval)
	eventSummaryCounter.Start(ctx)

	// Check if we have any node mappings
	if len(cfg.NodeMappings) == 0 {
		app.shutdown(fmt.Errorf("no node mappings configured - use --config, --node flags, or environment variables"))
		return
	}

	log.Printf("Using %d node mappings", len(cfg.NodeMappings))
	nodes := cfg.NodeMappings

	if cfg.SubscribeToTimeNode {
		log.Print("Subscribing to server time node")
		timeNodeConfig := config.NodeMapping{
			NodeName:   timeNode,
			MetricName: timeNodeMetricName,
		}
		nodes = append(nodes, timeNodeConfig)
	}

	client, err := setupOPCUAClient(ctx, cfg)
	if err != nil {
		app.shutdown(err)
		return
	}
	app.client = client
	defer client.Close(ctx)

	metricsRegistry := metrics.NewRegistry()
	if err := metricsRegistry.CreateFromNodeMappings(nodes, cfg.PromPrefix, cfg.Debug); err != nil {
		app.shutdown(fmt.Errorf("error creating metrics: %w", err))
		return
	}

	go func() {
		if err := setupMonitor(ctx, client, metricsRegistry.GetHandlerMap(), cfg.BufferSize, cfg.ReadTimeout, cfg.MaxTimeouts, cfg.Debug); err != nil {
			app.shutdown(fmt.Errorf("error setting up monitor: %w", err))
		}
	}()

	app.server = createHTTPServer(cfg.Port)
	log.Printf("Serving metrics on :%d", cfg.Port)
	if err := app.server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
		app.shutdown(fmt.Errorf("HTTP server error: %w", err))
	}
}

func getClient(endpoint *string) (*opcua.Client, error) {
	client, err := opcua.NewClient(*endpoint)
	if err != nil {
		return nil, fmt.Errorf("failed to create OPC UA client: %w", err)
	}
	return client, nil
}

// Subscribe to all the nodes and update the appropriate prometheus metrics on change
func setupMonitor(ctx context.Context, client *opcua.Client, handlerMap map[string][]metrics.HandlerRecord, bufferSize int, readTimeout time.Duration, maxTimeouts int, debug bool) error {
	m, err := monitor.NewNodeMonitor(client)
	if err != nil {
		return fmt.Errorf("failed to create node monitor: %w", err)
	}
	m.SetErrorHandler(handleError)

	var nodeList []string
	for nodeName := range handlerMap { // Node names are keys of handlerMap
		nodeList = append(nodeList, nodeName)
	}

	ch := make(chan *monitor.DataChangeMessage, bufferSize)
	params := opcua.SubscriptionParameters{Interval: time.Second}
	sub, err := m.ChanSubscribe(ctx, &params, ch, nodeList...)
	if err != nil {
		return fmt.Errorf("failed to create subscription: %w", err)
	}
	defer cleanup(ctx, sub)

	timeoutCount := 0
	for {
		uptimeGauge.Set(time.Since(startTime).Seconds())
		select {
		case <-ctx.Done():
			return nil
		case msg := <-ch:
			if msg.Error != nil {
				log.Printf("[error ] sub=%d error=%s", sub.SubscriptionID(), msg.Error)
			} else if msg.Value == nil {
				log.Printf("nil value received for node %s", msg.NodeID)
			} else {
				if debug {
					log.Printf("[message ] sub=%d ts=%s node=%s value=%v", sub.SubscriptionID(), msg.SourceTimestamp.UTC().Format(time.RFC3339), msg.NodeID, msg.Value.Value())
				}

				messageCounter.Inc()
				nodeID := msg.NodeID.String()
				eventSummaryCounter.Inc(nodeID)

				handleMessage(msg, handlerMap, debug)
			}
		case <-time.After(readTimeout):
			timeoutCount++
			log.Printf("Timeout %d wating for subscription messages", timeoutCount)
			if maxTimeouts > 0 && timeoutCount >= maxTimeouts {
				return fmt.Errorf("max timeouts (%d) exceeded", maxTimeouts)
			}
		}
	}

}

func cleanup(ctx context.Context, sub *monitor.Subscription) {
	log.Printf("stats: sub=%d delivered=%d dropped=%d", sub.SubscriptionID(), sub.Delivered(), sub.Dropped())
	sub.Unsubscribe(ctx)
}

func handleError(c *opcua.Client, sub *monitor.Subscription, err error) {
	log.Printf("[error] sub=%d error=%s", sub.SubscriptionID(), err)
	
	// Check for server halted or connection errors that might require reconnection
	errorStr := err.Error()
	if strings.Contains(errorStr, "StatusBadServerHalted") ||
	   strings.Contains(errorStr, "BadConnectionClosed") ||
	   strings.Contains(errorStr, "connection reset") {
		log.Printf("[error] Server connection issue detected: %s. Monitor will attempt to reconnect.", errorStr)
		// The monitor.NodeMonitor will handle reconnection automatically
		// We just log the issue for monitoring purposes
	}
}

func handleMessage(msg *monitor.DataChangeMessage, handlerMap map[string][]metrics.HandlerRecord, debug bool) {
	nodeID := msg.NodeID.String()
	for _, handlerMapRec := range handlerMap[nodeID] {
		handler := handlerMapRec.Handler
		value := msg.Value
		if debug {
			log.Printf("Handling %s --> %s", nodeID, handlerMapRec.Config.MetricName)
		}
		err := handler.Handle(*value)
		if err != nil {
			log.Printf("Error handling opcua value: %s (%s)\n", err, handlerMapRec.Config.MetricName)
		}
	}
}

