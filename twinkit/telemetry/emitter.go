package telemetry

import (
	"bytes"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"sync"
	"sync/atomic"
	"time"
)

const (
	defaultFlushInterval = 30 * time.Second
	defaultBatchSize     = 100
	defaultMaxRetries    = 3
	defaultRetryBackoff  = 2 * time.Second
)

// DeliveryMode controls how telemetry events are delivered to the collector.
type DeliveryMode string

const (
	// DeliveryBestEffort is fire-and-forget: non-blocking, no retries, loss acceptable.
	DeliveryBestEffort DeliveryMode = "best_effort"
	// DeliveryReliable uses a local disk buffer for at-least-once delivery with retries.
	DeliveryReliable DeliveryMode = "reliable"
)

// Emitter collects telemetry events and flushes them to a collector endpoint.
// In best-effort mode: fire-and-forget, never blocks, never retries, loss acceptable.
// In reliable mode: WAL-buffered, at-least-once with retries and exponential backoff.
type Emitter struct {
	mu            sync.Mutex
	enabled       bool
	collectorURL  string
	ingestKey     string
	orgID         string
	twinName      string
	twinVersion   string
	instanceID    string
	seq           atomic.Int64
	batch         []Event
	client        *http.Client
	stopCh        chan struct{}
	flushInterval time.Duration
	batchSize     int
	delivery      DeliveryMode
	bufferPath    string
	maxRetries    int
	retryBackoff  time.Duration
}

// Config configures the emitter.
type Config struct {
	Enabled       bool
	CollectorURL  string        // e.g., "https://telemetry.wondertwin.ai"
	IngestKey     string        // Bearer token for collector
	OrgID         string        // "community" for free tier
	TwinName      string
	TwinVersion   string
	FlushInterval time.Duration // default 30s
	BatchSize     int           // default 100
	Delivery      DeliveryMode  // default best_effort
	BufferPath    string        // local disk buffer for reliable mode
	MaxRetries    int           // retry count for reliable mode (default 3)
	RetryBackoff  time.Duration // initial backoff between retries (default 2s)
}

// NewEmitter creates a new telemetry emitter.
func NewEmitter(cfg Config) *Emitter {
	flushInterval := cfg.FlushInterval
	if flushInterval == 0 {
		flushInterval = defaultFlushInterval
	}
	batchSize := cfg.BatchSize
	if batchSize == 0 {
		batchSize = defaultBatchSize
	}
	delivery := cfg.Delivery
	if delivery == "" {
		delivery = DeliveryBestEffort
	}
	maxRetries := cfg.MaxRetries
	if maxRetries == 0 {
		maxRetries = defaultMaxRetries
	}
	retryBackoff := cfg.RetryBackoff
	if retryBackoff == 0 {
		retryBackoff = defaultRetryBackoff
	}
	return &Emitter{
		enabled:       cfg.Enabled,
		collectorURL:  cfg.CollectorURL,
		ingestKey:     cfg.IngestKey,
		orgID:         cfg.OrgID,
		twinName:      cfg.TwinName,
		twinVersion:   cfg.TwinVersion,
		instanceID:    generateInstanceID(),
		batch:         make([]Event, 0, batchSize),
		client:        &http.Client{Timeout: 10 * time.Second},
		flushInterval: flushInterval,
		batchSize:     batchSize,
		delivery:      delivery,
		bufferPath:    cfg.BufferPath,
		maxRetries:    maxRetries,
		retryBackoff:  retryBackoff,
	}
}

// Start begins the background flush loop.
// In reliable mode, also recovers any buffered events from a previous run.
func (e *Emitter) Start() {
	if !e.enabled || e.collectorURL == "" {
		return
	}
	e.stopCh = make(chan struct{})

	// Recover buffered events from disk (reliable mode only).
	if e.delivery == DeliveryReliable && e.bufferPath != "" {
		go e.recoverBuffer()
	}

	go e.flushLoop()
}

// Stop drains the batch and stops the flush loop.
func (e *Emitter) Stop() {
	if e.stopCh == nil {
		return
	}
	close(e.stopCh)
	e.flush() // drain remaining events
}

// Emit appends an event to the batch. Non-blocking.
// Assigns an event ID if not already set.
// If the batch reaches capacity, triggers an immediate flush.
func (e *Emitter) Emit(ev Event) {
	if !e.enabled {
		return
	}
	ev.Twin = e.twinName
	ev.TwinVersion = e.twinVersion
	ev.OrgID = e.orgID
	ev.InstanceID = e.instanceID
	ev.Seq = e.seq.Add(1)
	if ev.Timestamp == 0 {
		ev.Timestamp = time.Now().Unix()
	}
	if ev.EventID == "" {
		ev.EventID = generateEventID()
	}

	e.mu.Lock()
	e.batch = append(e.batch, ev)
	shouldFlush := len(e.batch) >= e.batchSize
	e.mu.Unlock()

	if shouldFlush {
		go e.flush()
	}
}

// EmitHTTP creates and emits an HTTP observation event.
func (e *Emitter) EmitHTTP(method, path string, status int, durationMS int64, reqBody, respBody []byte) {
	if !e.enabled {
		return
	}
	reqStats := ExtractBodyShapeWithStats(reqBody)
	respStats := ExtractBodyShapeWithStats(respBody)
	e.Emit(Event{
		EventType: EventHTTPObservation,
		Timestamp: time.Now().Unix(),
		Payload: HTTPObservation{
			Method:             method,
			PathTemplate:       TemplatePath(path),
			Status:             status,
			DurationMS:         durationMS,
			RequestBodyShape:   reqStats.Shape,
			ResponseBodyShape:  respStats.Shape,
			RequestBodyDepth:   reqStats.Depth,
			ResponseBodyDepth:  respStats.Depth,
			RequestFieldCount:  reqStats.FieldCount,
			ResponseFieldCount: respStats.FieldCount,
		},
	})
}

// EmitHTTPWithContext creates and emits an HTTP observation event with correlation ID.
func (e *Emitter) EmitHTTPWithContext(correlationID, method, path string, status int, durationMS int64, reqBody, respBody []byte) {
	if !e.enabled {
		return
	}
	reqStats := ExtractBodyShapeWithStats(reqBody)
	respStats := ExtractBodyShapeWithStats(respBody)
	e.Emit(Event{
		EventType:     EventHTTPObservation,
		Timestamp:     time.Now().Unix(),
		CorrelationID: correlationID,
		Payload: HTTPObservation{
			Method:             method,
			PathTemplate:       TemplatePath(path),
			Status:             status,
			DurationMS:         durationMS,
			RequestBodyShape:   reqStats.Shape,
			ResponseBodyShape:  respStats.Shape,
			RequestBodyDepth:   reqStats.Depth,
			ResponseBodyDepth:  respStats.Depth,
			RequestFieldCount:  reqStats.FieldCount,
			ResponseFieldCount: respStats.FieldCount,
		},
	})
}

// EmitDomain creates and emits a domain event (legacy signature for backward compatibility).
func (e *Emitter) EmitDomain(engine, hook, entityType, stateFrom, stateTo string) {
	if !e.enabled {
		return
	}
	e.Emit(Event{
		EventType: EventDomainEvent,
		Timestamp: time.Now().Unix(),
		Payload: DomainEvent{
			Engine:     engine,
			Hook:       hook,
			EntityType: entityType,
			StateFrom:  stateFrom,
			StateTo:    stateTo,
		},
	})
}

// EmitDomainEvent emits a fully-specified domain event with enriched context.
func (e *Emitter) EmitDomainEvent(correlationID string, de DomainEvent) {
	if !e.enabled {
		return
	}
	e.Emit(Event{
		EventType:     EventDomainEvent,
		Timestamp:     time.Now().Unix(),
		CorrelationID: correlationID,
		Payload:       de,
	})
}

// EmitScenario creates and emits a scenario result event.
func (e *Emitter) EmitScenario(result ScenarioResult) {
	if !e.enabled {
		return
	}
	e.Emit(Event{
		EventType: EventScenarioResult,
		Timestamp: time.Now().Unix(),
		Payload:   result,
	})
}

// Enable turns on telemetry emission (called when admin config is updated).
func (e *Emitter) Enable(collectorURL, ingestKey, orgID string) {
	e.mu.Lock()
	e.enabled = true
	e.collectorURL = collectorURL
	e.ingestKey = ingestKey
	e.orgID = orgID
	e.mu.Unlock()
	e.Start()
}

// Enabled returns whether the emitter is active.
func (e *Emitter) Enabled() bool {
	e.mu.Lock()
	defer e.mu.Unlock()
	return e.enabled
}

// BatchLen returns the current batch size (for testing).
func (e *Emitter) BatchLen() int {
	e.mu.Lock()
	defer e.mu.Unlock()
	return len(e.batch)
}

// DeliveryConfig returns the current delivery mode (for testing/diagnostics).
func (e *Emitter) DeliveryConfig() DeliveryMode {
	return e.delivery
}

// InstanceID returns the emitter's unique instance identifier.
func (e *Emitter) InstanceID() string {
	return e.instanceID
}

func (e *Emitter) flushLoop() {
	ticker := time.NewTicker(e.flushInterval)
	defer ticker.Stop()
	for {
		select {
		case <-ticker.C:
			e.flush()
		case <-e.stopCh:
			return
		}
	}
}

func (e *Emitter) flush() {
	e.mu.Lock()
	if len(e.batch) == 0 {
		e.mu.Unlock()
		return
	}
	batch := e.batch
	e.batch = make([]Event, 0, e.batchSize)
	e.mu.Unlock()

	if e.delivery == DeliveryReliable {
		e.flushReliable(batch)
	} else {
		e.flushBestEffort(batch)
	}
}

// flushBestEffort sends the batch once without retries. Loss is acceptable.
func (e *Emitter) flushBestEffort(batch []Event) {
	if err := e.send(batch); err != nil {
		log.Printf("telemetry: flush error: %v", err)
	}
}

// flushReliable writes the batch to a WAL buffer on disk, then attempts delivery
// with retries and exponential backoff. Events survive twin restarts.
func (e *Emitter) flushReliable(batch []Event) {
	// Write to disk buffer first (WAL).
	bufferFile := e.writeBuffer(batch)

	// Attempt delivery with retries.
	backoff := e.retryBackoff
	for attempt := 0; attempt <= e.maxRetries; attempt++ {
		if err := e.send(batch); err != nil {
			log.Printf("telemetry: reliable flush attempt %d/%d failed: %v", attempt+1, e.maxRetries+1, err)
			if attempt < e.maxRetries {
				time.Sleep(backoff)
				backoff *= 2 // exponential backoff
			}
			continue
		}
		// Success — remove the buffer file.
		if bufferFile != "" {
			os.Remove(bufferFile)
		}
		return
	}
	// All retries exhausted. Buffer file remains on disk for recovery on next start.
	log.Printf("telemetry: reliable flush exhausted retries, %d events buffered to %s", len(batch), bufferFile)
}

// send performs the HTTP POST to the collector. Returns error on failure.
func (e *Emitter) send(batch []Event) error {
	body, err := json.Marshal(batch)
	if err != nil {
		return fmt.Errorf("marshal: %w", err)
	}

	req, err := http.NewRequest("POST", e.collectorURL+"/telemetry/v1/ingest", bytes.NewReader(body))
	if err != nil {
		return fmt.Errorf("request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	if e.ingestKey != "" {
		req.Header.Set("Authorization", "Bearer "+e.ingestKey)
	}

	resp, err := e.client.Do(req)
	if err != nil {
		return fmt.Errorf("http: %w", err)
	}
	resp.Body.Close()

	if resp.StatusCode >= 400 {
		return fmt.Errorf("collector returned %d", resp.StatusCode)
	}
	return nil
}

// writeBuffer writes a batch to a WAL file on disk. Returns the file path.
func (e *Emitter) writeBuffer(batch []Event) string {
	if e.bufferPath == "" {
		return ""
	}
	if err := os.MkdirAll(e.bufferPath, 0o755); err != nil {
		log.Printf("telemetry: buffer mkdir error: %v", err)
		return ""
	}
	filename := filepath.Join(e.bufferPath, fmt.Sprintf("wal_%d_%s.json", time.Now().UnixNano(), generateShortID()))
	data, err := json.Marshal(batch)
	if err != nil {
		log.Printf("telemetry: buffer marshal error: %v", err)
		return ""
	}
	if err := os.WriteFile(filename, data, 0o644); err != nil {
		log.Printf("telemetry: buffer write error: %v", err)
		return ""
	}
	return filename
}

// recoverBuffer reads and re-sends any WAL files from a previous run.
func (e *Emitter) recoverBuffer() {
	if e.bufferPath == "" {
		return
	}
	entries, err := os.ReadDir(e.bufferPath)
	if err != nil {
		return // no buffer directory
	}
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		path := filepath.Join(e.bufferPath, entry.Name())
		data, err := os.ReadFile(path)
		if err != nil {
			log.Printf("telemetry: recover read error: %v", err)
			continue
		}
		var batch []Event
		if err := json.Unmarshal(data, &batch); err != nil {
			log.Printf("telemetry: recover unmarshal error: %v", err)
			os.Remove(path) // corrupt file, discard
			continue
		}
		if err := e.send(batch); err != nil {
			log.Printf("telemetry: recover send error: %v", err)
			continue // leave file for next attempt
		}
		os.Remove(path) // success, clean up
	}
}

// generateEventID produces a unique event ID for deduplication.
func generateEventID() string {
	b := make([]byte, 8)
	if _, err := rand.Read(b); err != nil {
		return fmt.Sprintf("evt_%d", time.Now().UnixNano())
	}
	return "evt_" + hex.EncodeToString(b)
}

// generateShortID produces a short random hex string for WAL filenames.
func generateShortID() string {
	b := make([]byte, 4)
	rand.Read(b)
	return hex.EncodeToString(b)
}

// generateInstanceID produces a unique ID for this emitter lifecycle.
func generateInstanceID() string {
	b := make([]byte, 8)
	if _, err := rand.Read(b); err != nil {
		return fmt.Sprintf("inst_%d", time.Now().UnixNano())
	}
	return "inst_" + hex.EncodeToString(b)
}
