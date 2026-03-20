package telemetry

import (
	"bytes"
	"encoding/json"
	"log"
	"net/http"
	"sync"
	"time"
)

const (
	defaultFlushInterval = 30 * time.Second
	defaultBatchSize     = 100
)

// Emitter collects telemetry events and flushes them to a collector endpoint.
// Fire-and-forget — never blocks, never retries, loss is acceptable.
type Emitter struct {
	mu            sync.Mutex
	enabled       bool
	collectorURL  string
	ingestKey     string
	orgID         string
	twinName      string
	twinVersion   string
	batch         []Event
	client        *http.Client
	stopCh        chan struct{}
	flushInterval time.Duration
	batchSize     int
}

// Config configures the emitter.
type Config struct {
	Enabled       bool
	CollectorURL  string // e.g., "https://telemetry.wondertwin.ai"
	IngestKey     string // Bearer token for collector
	OrgID         string // "community" for free tier
	TwinName      string
	TwinVersion   string
	FlushInterval time.Duration // default 30s
	BatchSize     int           // default 100
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
	return &Emitter{
		enabled:       cfg.Enabled,
		collectorURL:  cfg.CollectorURL,
		ingestKey:     cfg.IngestKey,
		orgID:         cfg.OrgID,
		twinName:      cfg.TwinName,
		twinVersion:   cfg.TwinVersion,
		batch:         make([]Event, 0, batchSize),
		client:        &http.Client{Timeout: 10 * time.Second},
		flushInterval: flushInterval,
		batchSize:     batchSize,
	}
}

// Start begins the background flush loop.
func (e *Emitter) Start() {
	if !e.enabled || e.collectorURL == "" {
		return
	}
	e.stopCh = make(chan struct{})
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
// If the batch reaches capacity, triggers an immediate flush.
func (e *Emitter) Emit(ev Event) {
	if !e.enabled {
		return
	}
	ev.Twin = e.twinName
	ev.TwinVersion = e.twinVersion
	ev.OrgID = e.orgID
	if ev.Timestamp == 0 {
		ev.Timestamp = time.Now().Unix()
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
	e.Emit(Event{
		EventType: EventHTTPObservation,
		Timestamp: time.Now().Unix(),
		Payload: HTTPObservation{
			Method:            method,
			PathTemplate:      TemplatePath(path),
			Status:            status,
			DurationMS:        durationMS,
			RequestBodyShape:  ExtractBodyShape(reqBody),
			ResponseBodyShape: ExtractBodyShape(respBody),
		},
	})
}

// EmitDomain creates and emits a domain event.
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

	body, err := json.Marshal(batch)
	if err != nil {
		log.Printf("telemetry: marshal error: %v", err)
		return
	}

	req, err := http.NewRequest("POST", e.collectorURL+"/telemetry/v1/ingest", bytes.NewReader(body))
	if err != nil {
		log.Printf("telemetry: request error: %v", err)
		return
	}
	req.Header.Set("Content-Type", "application/json")
	if e.ingestKey != "" {
		req.Header.Set("Authorization", "Bearer "+e.ingestKey)
	}

	resp, err := e.client.Do(req)
	if err != nil {
		log.Printf("telemetry: flush error: %v", err)
		return
	}
	resp.Body.Close()
}
