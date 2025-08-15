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
	"github.com/mwieczorkiewicz/opcua_exporter/internal/handlers"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promhttp"
)

const (
	timeNode           string = "i=2258"            // Server time node ID
	timeNodeMetricName string = "opcua_server_time" // Metric name for server time
)


// NodeConfig : Structure for representing OPCUA nodes to monitor.
type NodeConfig = config.NodeMapping

// MsgHandler interface can convert OPC UA Variant objects
// and emit prometheus metrics
type MsgHandler = handlers.MsgHandler

// HandlerMap maps OPC UA channel names to MsgHandlers
type HandlerMap = map[string][]handlerMapRecord

type handlerMapRecord struct {
	config  NodeConfig
	handler MsgHandler
}

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
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		app.server.Shutdown(ctx)
	}
	app.cancel()
	os.Exit(1)
}

func init() {
	subsystem := "opcua_exporter"
	uptimeGauge = prometheus.NewGauge(prometheus.GaugeOpts{
		Subsystem: subsystem,
		Name:      "uptime_seconds",
		Help:      "Time in seconds since the OPCUA exporter started",
	})
	uptimeGauge.Set(time.Since(startTime).Seconds())
	prometheus.MustRegister(uptimeGauge)

	messageCounter = prometheus.NewCounter(prometheus.CounterOpts{
		Subsystem: subsystem,
		Name:      "message_count",
		Help:      "Total number of OPCUA channel updates received by the exporter",
	})
	prometheus.MustRegister(messageCounter)
}

func main() {
	log.Print("Starting up.")
	
	// Initialize viper configuration with flags and environment variables
	pflag.Int("port", 9686, "Port to publish metrics on.")
	pflag.String("endpoint", "opc.tcp://localhost:4096", "OPC UA Endpoint to connect to.")
	pflag.String("prom-prefix", "", "Prefix will be appended to emitted prometheus metrics")
	pflag.String("config", "", "Path to a file from which to read the list of OPC UA nodes to monitor")
	pflag.String("config-b64", "", "Base64-encoded config JSON. Overrides -config")
	pflag.Bool("debug", false, "Enable debug logging")
	pflag.Duration("read-timeout", 5*time.Second, "Timeout when waiting for OPCUA subscription messages")
	pflag.Int("max-timeouts", 0, "The exporter will quit trying after this many read timeouts (0 to disable).")
	pflag.Int("buffer-size", 64, "Maximum number of messages in the receive buffer")
	pflag.Duration("summary-interval", 5*time.Minute, "How frequently to print an event count summary")
	pflag.Bool("subscribe-to-time-node", false, "Subscribe to the server time node and emit it as a gauge metric")
	pflag.StringArray("node", []string{}, "Node mapping in format 'nodeId,metricName[,extractBit]' (can be repeated)")
	pflag.Parse()

	viper.BindPFlags(pflag.CommandLine)
	cfg := config.InitViper()
	
	// Parse node flag values and add them to configuration
	nodeFlags := viper.GetStringSlice("node")
	for _, nodeFlag := range nodeFlags {
		parts := strings.Split(nodeFlag, ",")
		if len(parts) < 2 {
			log.Printf("Invalid node flag format '%s', expected 'nodeId,metricName[,extractBit]'", nodeFlag)
			continue
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
					log.Printf("Invalid extractBit value '%s' in node flag '%s'", extractBitStr, nodeFlag)
					continue
				}
				nodeMapping.ExtractBit = extractBit
			}
		}
		
		cfg.NodeMappings = append(cfg.NodeMappings, nodeMapping)
		log.Printf("Added node mapping from flag: %s -> %s", nodeMapping.NodeName, nodeMapping.MetricName)
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

	var nodes []NodeConfig
	var readError error
	
	// Priority order: 1. Direct node mappings in config, 2. Base64 config, 3. Config file
	if len(cfg.NodeMappings) > 0 {
		log.Printf("Using %d node mappings from configuration", len(cfg.NodeMappings))
		nodes = cfg.NodeMappings
	} else if cfg.ConfigB64 != "" {
		log.Print("Using base64-encoded config")
		nodes, readError = config.ReadBase64(&cfg.ConfigB64)
	} else if cfg.ConfigFile != "" {
		log.Printf("Reading config from %s", cfg.ConfigFile)
		nodes, readError = config.ReadFile(cfg.ConfigFile)
	} else {
		app.shutdown(fmt.Errorf("requires node mappings via config file, base64 config, YAML config, or environment variables"))
		return
	}

	if readError != nil {
		app.shutdown(fmt.Errorf("error reading config: %w", readError))
		return
	}

	if cfg.SubscribeToTimeNode {
		log.Print("Subscribing to server time node")
		timeNode := NodeConfig{
			NodeName:   timeNode,
			MetricName: timeNodeMetricName,
		}
		nodes = append(nodes, timeNode)
	}

	client, err := getClient(&cfg.Endpoint)
	if err != nil {
		app.shutdown(fmt.Errorf("error creating OPC UA client: %w", err))
		return
	}
	app.client = client

	log.Printf("Connecting to OPCUA server at %s", cfg.Endpoint)
	if err := client.Connect(ctx); err != nil {
		app.shutdown(fmt.Errorf("error connecting to OPC UA client: %w", err))
		return
	}
	log.Print("Connected successfully")
	defer client.Close(ctx)

	metricMap, err := createMetrics(&nodes)
	if err != nil {
		app.shutdown(fmt.Errorf("error creating metrics: %w", err))
		return
	}

	go func() {
		if err := setupMonitor(ctx, client, metricMap, cfg.BufferSize, cfg.ReadTimeout, cfg.MaxTimeouts, cfg.Debug); err != nil {
			app.shutdown(fmt.Errorf("error setting up monitor: %w", err))
		}
	}()

	http.Handle("/metrics", promhttp.Handler())
	var listenOn = fmt.Sprintf(":%d", cfg.Port)

	app.server = &http.Server{
		Addr:         listenOn,
		Handler:      nil,
		ReadTimeout:  15 * time.Second,
		WriteTimeout: 15 * time.Second,
		IdleTimeout:  60 * time.Second,
	}

	log.Printf("Serving metrics on %s", listenOn)
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
func setupMonitor(ctx context.Context, client *opcua.Client, handlerMap HandlerMap, bufferSize int, readTimeout time.Duration, maxTimeouts int, debug bool) error {
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

	lag := time.Millisecond * 10
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
			time.Sleep(lag)
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
}

func handleMessage(msg *monitor.DataChangeMessage, handlerMap HandlerMap, debug bool) {
	nodeID := msg.NodeID.String()
	for _, handlerMapRec := range handlerMap[nodeID] {
		handler := handlerMapRec.handler
		value := msg.Value
		if debug {
			log.Printf("Handling %s --> %s", nodeID, handlerMapRec.config.MetricName)
		}
		err := handler.Handle(*value)
		if err != nil {
			log.Printf("Error handling opcua value: %s (%s)\n", err, handlerMapRec.config.MetricName)
		}
	}
}

// Initialize a Prometheus gauge for each node. Return them as a map.
func createMetrics(nodeConfigs *[]NodeConfig) (HandlerMap, error) {
	handlerMap := make(HandlerMap)
	for _, nodeConfig := range *nodeConfigs {
		nodeName := nodeConfig.NodeName
		metricName := nodeConfig.MetricName
		handler, err := createHandler(nodeConfig)
		if err != nil {
			return nil, fmt.Errorf("error creating handler for %s: %w", metricName, err)
		}
		mapRecord := handlerMapRecord{nodeConfig, handler}
		handlerMap[nodeName] = append(handlerMap[nodeName], mapRecord)
		log.Printf("Created prom metric %s for OPC UA node %s", metricName, nodeName)
	}

	return handlerMap, nil
}

func createHandler(nodeConfig NodeConfig) (MsgHandler, error) {
	metricName := nodeConfig.MetricName
	if metricName == "" {
		return nil, fmt.Errorf("metric name cannot be empty")
	}
	if viper.GetString("prom-prefix") != "" {
		metricName = fmt.Sprintf("%s_%s", viper.GetString("prom-prefix"), metricName)
	}
	g := prometheus.NewGauge(prometheus.GaugeOpts{
		Name: metricName,
		Help: "From OPC UA",
	})
	if err := prometheus.Register(g); err != nil {
		return nil, fmt.Errorf("failed to register metric %s: %w", metricName, err)
	}

	var handler MsgHandler
	if nodeConfig.ExtractBit != nil {
		extractBit, ok := nodeConfig.ExtractBit.(int)
		if !ok {
			return nil, fmt.Errorf("extractBit must be an integer, got %T", nodeConfig.ExtractBit)
		}
		handler = handlers.NewOpcuaBitVectorHandler(g, extractBit, viper.GetBool("debug"))
	} else {
		handler = handlers.NewOpcValueHandler(g)
	}
	return handler, nil
}
