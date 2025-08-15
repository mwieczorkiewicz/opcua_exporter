opcua_exporter
==============

**Enhanced fork of [open-strateos/opcua_exporter](https://github.com/open-strateos/opcua_exporter) with modern improvements**

This is a Prometheus exporter for the [OPC Unified Architecture](https://en.wikipedia.org/wiki/OPC_Unified_Architecture) protocol with significant enhancements:

🚀 Improvements Over Original
-----------------------------

- **Modern Go dependencies** - Updated to Go 1.23+ with latest dependency versions
- **Unified configuration system** - Powered by [spf13/viper](https://github.com/spf13/viper) for flexible configuration
- **Multiple configuration methods** - Command line flags, environment variables, YAML config, and legacy support
- **Enhanced node mapping** - Direct node configuration without separate files
- **Improved maintainability** - Better code organization and comprehensive testing

It uses [gopcua/opcua](https://github.com/gopcua/opcua) to communicate with an OPCUA endpoint, subscribes to
selected channels, and republishes them as Promtheus metrics on a port of your choice.

Usage
-----

```shell
Usage of opcua_exporter:
      --buffer-size int             Maximum number of messages in the receive buffer (default 64)
      --config string               Path to a file from which to read the list of OPC UA nodes to monitor
      --config-b64 string           Base64-encoded config JSON. Overrides -config
      --debug                       Enable debug logging
      --endpoint string             OPC UA Endpoint to connect to. (default "opc.tcp://localhost:4096")
      --max-timeouts int            The exporter will quit trying after this many read timeouts (0 to disable).
      --node stringArray            Node mapping in format 'nodeId,metricName[,extractBit]' (can be repeated)
      --port int                    Port to publish metrics on. (default 9686)
      --prom-prefix string          Prefix will be appended to emitted prometheus metrics
      --read-timeout duration       Timeout when waiting for OPCUA subscription messages (default 5s)
      --subscribe-to-time-node      Subscribe to the server time node and emit it as a gauge metric
      --summary-interval duration   How frequently to print an event count summary (default 5m0s)
```

Configuration Methods
---------------------

This exporter supports multiple ways to configure node mappings, in order of precedence:

1. **Command line flags** (highest priority)
2. **Environment variables**
3. **YAML configuration file**
4. **Legacy config files** (lowest priority)

----------------------------

1. Command Line Flags (NEW)

----------------------------

Configure nodes directly via command line:

```bash
# Simple node mapping
./opcua_exporter --node "ns=1;s=Voltmeter,circuit_input_volts"

# Multiple nodes with bit extraction
./opcua_exporter \
  --node "ns=1;s=Voltmeter,circuit_input_volts" \
  --node "ns=1;s=Ammeter,circuit_input_amps" \
  --node "ns=1;s=CircuitBreakerStates,circuit_breaker_three_tripped,3"
```

1. Environment Variables (NEW)

-------------------------------;

Configure via environment variables with indexed format:

```bash
# Basic configuration
export OPCUA_EXPORTER_PORT=9090
export OPCUA_EXPORTER_ENDPOINT="opc.tcp://plc.example.com:4840"
export OPCUA_EXPORTER_DEBUG=true

# Node mappings (supports up to 100 nodes)
export OPCUA_EXPORTER_NODES_0_NODENAME="ns=1;s=Temperature"
export OPCUA_EXPORTER_NODES_0_METRICNAME="temperature_celsius"

export OPCUA_EXPORTER_NODES_1_NODENAME="ns=1;s=AlarmBits"
export OPCUA_EXPORTER_NODES_1_METRICNAME="alarm_high_temp"
export OPCUA_EXPORTER_NODES_1_EXTRACTBIT="2"

./opcua_exporter
```

1. YAML Configuration File (ENHANCED)

----------------------------

Create `opcua_exporter.yaml` in current directory, `$HOME/.opcua_exporter/`, or `/etc/opcua_exporter/`:

```yaml
port: 9090
endpoint: "opc.tcp://plc.example.com:4840"
debug: true
prom-prefix: "factory"
read-timeout: "10s"
subscribe-to-time-node: true

# Node mappings directly in config file
nodes:
  - nodeName: "ns=1;s=Temperature"
    metricName: "temperature_celsius"
  - nodeName: "ns=1;s=Pressure"
    metricName: "pressure_bar"
  - nodeName: "ns=1;s=AlarmBits"
    metricName: "alarm_high_temp"
    extractBit: 2
```

1. Legacy Configuration Files
------------------------------;

Legacy support for separate node configuration files:

```bash
# Via config file
./opcua_exporter --config nodes.yaml

# Via base64-encoded config
./opcua_exporter --config-b64 "$(base64 < nodes.yaml)"
```

Environment Variables Reference
--------------------------------

All configuration options can be set via environment variables with the `OPCUA_EXPORTER_` prefix:

| Environment Variable | Flag Equivalent | Description | Default |
|---------------------|-----------------|-------------|---------|
| `OPCUA_EXPORTER_PORT` | `--port` | Port to publish metrics on | `9686` |
| `OPCUA_EXPORTER_ENDPOINT` | `--endpoint` | OPC UA Endpoint to connect to | `opc.tcp://localhost:4096` |
| `OPCUA_EXPORTER_PROM_PREFIX` | `--prom-prefix` | Prefix for emitted prometheus metrics | _(empty)_ |
| `OPCUA_EXPORTER_CONFIG` | `--config` | Path to node config file | _(empty)_ |
| `OPCUA_EXPORTER_CONFIG_B64` | `--config-b64` | Base64-encoded config | _(empty)_ |
| `OPCUA_EXPORTER_DEBUG` | `--debug` | Enable debug logging | `false` |
| `OPCUA_EXPORTER_READ_TIMEOUT` | `--read-timeout` | Timeout for subscription messages | `5s` |
| `OPCUA_EXPORTER_MAX_TIMEOUTS` | `--max-timeouts` | Max timeouts before quit | `0` (disabled) |
| `OPCUA_EXPORTER_BUFFER_SIZE` | `--buffer-size` | Message receive buffer size | `64` |
| `OPCUA_EXPORTER_SUMMARY_INTERVAL` | `--summary-interval` | Event count summary frequency | `5m` |
| `OPCUA_EXPORTER_SUBSCRIBE_TO_TIME_NODE` | `--subscribe-to-time-node` | Subscribe to server time node | `false` |

Node Mapping Environment Variables
-----------------------------------

For indexed node mappings, use the format `OPCUA_EXPORTER_NODES_{INDEX}_{FIELD}`:

- `OPCUA_EXPORTER_NODES_0_NODENAME` - Node identifier (required)
- `OPCUA_EXPORTER_NODES_0_METRICNAME` - Prometheus metric name (required)  
- `OPCUA_EXPORTER_NODES_0_EXTRACTBIT` - Bit index for bit extraction (optional)

Supports up to 100 nodes (indices 0-99).

Node Configuration
------------------

You need to supply a mapping of stringified OPC-UA node names to Prometheus metric names.
This is necessary because the OPC-UA node strings use `;` and `=` characters to separate
fields from each other and names from values, while Prometheus metric names must match
the regex `[a-zA-Z_:][a-zA-Z0-9_:]*`. For good advice on Prometheus metric naming, refer
to the [Prometheus docs](https://prometheus.io/docs/practices/naming/).

Legacy YAML Config File Format
-------------------------------

For legacy compatibility, you can still use separate node config files:

```yaml
- nodeName: ns=1;s=Voltmeter
  metricName: circuit_input_volts
- nodeName: ns=1;s=Ammeter
  metricName: circuit_input_amps
- nodeName: ns=1;s=CircuitBreakerStates
  extractBit: 3 # pull just this bit from a bit-vector channel
  metricName: circuit_breaker_three_tripped
```

Bit Vectors
-----------

Some OPC-UA devices send alarm states as binary bit-vector values,
for example, a 32-bit unsigned integer where each bit represents the state of a circuit breaker.

If you want to monitor a single bit, you can specify an `extractBit` in any configuration method.
The exporter will pull just that bit (zero-indexed) from the value of the OPC-UA channel, and export it
as a 0.0 or 1.0 Prometheus metric value.

Examples
--------

**Command Line:**

```bash
./opcua_exporter --node "ns=1;s=AlarmBits,circuit_breaker_alarm,5"
```

**Environment Variable:**

```bash
export OPCUA_EXPORTER_NODES_0_EXTRACTBIT="5"
```

**YAML Config:**

```yaml
nodes:
  - nodeName: "ns=1;s=AlarmBits"
    metricName: "circuit_breaker_alarm"
    extractBit: 5
```

Docker Usage
------------

```bash
# Build
docker build -t opcua_exporter .

# Run with environment variables
docker run -e OPCUA_EXPORTER_ENDPOINT="opc.tcp://host.docker.internal:4840" \
           -e OPCUA_EXPORTER_NODES_0_NODENAME="ns=1;s=Temperature" \
           -e OPCUA_EXPORTER_NODES_0_METRICNAME="temperature_celsius" \
           -p 9686:9686 opcua_exporter
```
