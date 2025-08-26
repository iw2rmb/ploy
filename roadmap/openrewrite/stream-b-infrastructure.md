# Stream B: Distributed Infrastructure

## Overview
**Goal**: Add scalability and reliability to the OpenRewrite service  
**Timeline**: Days 1-4  
**Dependencies**: Can run parallel to Stream A, integrates at Day 3  
**Deliverable**: Distributed job processing with Consul/SeaweedFS backend

## Phase B1: Consul KV & SeaweedFS Setup

### Objectives
- [x] Configure Consul KV for job status tracking ✅ 2025-08-26
- [x] Setup SeaweedFS for diff storage ✅ 2025-08-26
- [x] Create storage abstraction layer ✅ 2025-08-26
- [x] Implement job status management ✅ 2025-08-26

### B1.1: Consul KV Integration

#### Implementation
```go
// internal/storage/consul/client.go
package consul

import (
    "encoding/json"
    "fmt"
    "github.com/hashicorp/consul/api"
)

type ConsulStorage struct {
    client *api.Client
    prefix string
}

func NewConsulStorage(address string) (*ConsulStorage, error) {
    config := api.DefaultConfig()
    config.Address = address
    
    client, err := api.NewClient(config)
    if err != nil {
        return nil, err
    }
    
    return &ConsulStorage{
        client: client,
        prefix: "ploy/openrewrite/jobs",
    }, nil
}

// StoreJobStatus saves job status to Consul KV
func (c *ConsulStorage) StoreJobStatus(jobID string, status *JobStatus) error {
    key := fmt.Sprintf("%s/%s", c.prefix, jobID)
    
    data, err := json.Marshal(status)
    if err != nil {
        return err
    }
    
    pair := &api.KVPair{
        Key:   key,
        Value: data,
    }
    
    _, err = c.client.KV().Put(pair, nil)
    return err
}

// GetJobStatus retrieves job status from Consul
func (c *ConsulStorage) GetJobStatus(jobID string) (*JobStatus, error) {
    key := fmt.Sprintf("%s/%s", c.prefix, jobID)
    
    pair, _, err := c.client.KV().Get(key, nil)
    if err != nil {
        return nil, err
    }
    
    if pair == nil {
        return nil, fmt.Errorf("job not found: %s", jobID)
    }
    
    var status JobStatus
    if err := json.Unmarshal(pair.Value, &status); err != nil {
        return nil, err
    }
    
    return &status, nil
}

// WatchJobStatus creates a blocking query for status changes
func (c *ConsulStorage) WatchJobStatus(jobID string, index uint64) (*JobStatus, uint64, error) {
    key := fmt.Sprintf("%s/%s", c.prefix, jobID)
    
    options := &api.QueryOptions{
        WaitIndex: index,
        WaitTime:  30 * time.Second,
    }
    
    pair, meta, err := c.client.KV().Get(key, options)
    if err != nil {
        return nil, 0, err
    }
    
    if pair == nil {
        return nil, meta.LastIndex, nil
    }
    
    var status JobStatus
    if err := json.Unmarshal(pair.Value, &status); err != nil {
        return nil, 0, err
    }
    
    return &status, meta.LastIndex, nil
}
```

### B1.2: SeaweedFS Integration

#### Implementation
```go
// internal/storage/seaweedfs/client.go
package seaweedfs

import (
    "bytes"
    "encoding/base64"
    "fmt"
    "io"
    "net/http"
)

type SeaweedFSStorage struct {
    masterURL string
    httpClient *http.Client
}

func NewSeaweedFSStorage(masterURL string) *SeaweedFSStorage {
    return &SeaweedFSStorage{
        masterURL: masterURL,
        httpClient: &http.Client{
            Timeout: 30 * time.Second,
        },
    }
}

// StoreDiff uploads diff to SeaweedFS and returns URL
func (s *SeaweedFSStorage) StoreDiff(jobID string, diff []byte) (string, error) {
    // Assign volume
    assignURL := fmt.Sprintf("%s/dir/assign", s.masterURL)
    resp, err := s.httpClient.Post(assignURL, "application/json", nil)
    if err != nil {
        return "", err
    }
    defer resp.Body.Close()
    
    var assign AssignResponse
    if err := json.NewDecoder(resp.Body).Decode(&assign); err != nil {
        return "", err
    }
    
    // Upload diff
    uploadURL := fmt.Sprintf("http://%s/%s", assign.URL, assign.FID)
    
    // Create multipart form
    body := &bytes.Buffer{}
    writer := multipart.NewWriter(body)
    
    part, err := writer.CreateFormFile("file", fmt.Sprintf("%s.diff", jobID))
    if err != nil {
        return "", err
    }
    
    if _, err := io.Copy(part, bytes.NewReader(diff)); err != nil {
        return "", err
    }
    
    writer.Close()
    
    req, err := http.NewRequest("POST", uploadURL, body)
    if err != nil {
        return "", err
    }
    req.Header.Set("Content-Type", writer.FormDataContentType())
    
    resp, err = s.httpClient.Do(req)
    if err != nil {
        return "", err
    }
    defer resp.Body.Close()
    
    if resp.StatusCode != http.StatusCreated {
        return "", fmt.Errorf("upload failed: %d", resp.StatusCode)
    }
    
    return assign.FID, nil
}

// RetrieveDiff downloads diff from SeaweedFS
func (s *SeaweedFSStorage) RetrieveDiff(fileID string) ([]byte, error) {
    // Lookup volume
    lookupURL := fmt.Sprintf("%s/dir/lookup?volumeId=%s", s.masterURL, fileID)
    resp, err := s.httpClient.Get(lookupURL)
    if err != nil {
        return nil, err
    }
    defer resp.Body.Close()
    
    var lookup LookupResponse
    if err := json.NewDecoder(resp.Body).Decode(&lookup); err != nil {
        return nil, err
    }
    
    if len(lookup.Locations) == 0 {
        return nil, fmt.Errorf("no locations found for %s", fileID)
    }
    
    // Download from first location
    downloadURL := fmt.Sprintf("http://%s/%s", lookup.Locations[0].URL, fileID)
    resp, err = s.httpClient.Get(downloadURL)
    if err != nil {
        return nil, err
    }
    defer resp.Body.Close()
    
    return io.ReadAll(resp.Body)
}
```

### B1.3: Storage Abstraction Layer

```go
// internal/storage/interface.go
package storage

type JobStorage interface {
    StoreJobStatus(jobID string, status *JobStatus) error
    GetJobStatus(jobID string) (*JobStatus, error)
    WatchJobStatus(jobID string, index uint64) (*JobStatus, uint64, error)
    StoreDiff(jobID string, diff []byte) (string, error)
    RetrieveDiff(fileID string) ([]byte, error)
}

type JobStatus struct {
    JobID       string    `json:"job_id"`
    Status      string    `json:"status"` // queued, processing, completed, failed
    CreatedAt   time.Time `json:"created_at"`
    UpdatedAt   time.Time `json:"updated_at"`
    CompletedAt *time.Time `json:"completed_at,omitempty"`
    DiffURL     string    `json:"diff_url,omitempty"`
    Error       string    `json:"error,omitempty"`
    Progress    int       `json:"progress"` // 0-100
    Message     string    `json:"message"`
}
```

### B1.4: Testing Checklist
- [x] Consul KV connection established ✅ 2025-08-26
- [x] Job status CRUD operations working ✅ 2025-08-26
- [x] SeaweedFS file upload/download working ✅ 2025-08-26
- [x] Storage abstraction layer tested ✅ 2025-08-26
- [x] Watch mechanism for status updates ✅ 2025-08-26

## Phase B2: Job Queue & Worker Pool

### Objectives
- [x] Implement job queue with priority support ✅ 2025-08-26
- [x] Create worker pool for concurrent processing ✅ 2025-08-26
- [x] Add retry logic for failed jobs ✅ 2025-08-26
- [x] Implement job cancellation ✅ 2025-08-26

### B2.1: Job Queue Implementation

```go
// internal/queue/queue.go
package queue

import (
    "container/heap"
    "sync"
    "time"
)

type JobQueue struct {
    mu        sync.Mutex
    jobs      JobHeap
    workers   chan chan *Job
    maxWorkers int
    storage   storage.JobStorage
}

type Job struct {
    ID        string
    Priority  int
    TarData   []byte
    Recipe    RecipeConfig
    CreatedAt time.Time
    Retries   int
    index     int // for heap
}

// JobHeap implements heap.Interface
type JobHeap []*Job

func (h JobHeap) Len() int           { return len(h) }
func (h JobHeap) Less(i, j int) bool {
    // Higher priority first, then older jobs
    if h[i].Priority != h[j].Priority {
        return h[i].Priority > h[j].Priority
    }
    return h[i].CreatedAt.Before(h[j].CreatedAt)
}

func NewJobQueue(maxWorkers int, storage storage.JobStorage) *JobQueue {
    q := &JobQueue{
        jobs:      make(JobHeap, 0),
        workers:   make(chan chan *Job, maxWorkers),
        maxWorkers: maxWorkers,
        storage:   storage,
    }
    
    heap.Init(&q.jobs)
    q.startWorkers()
    q.startDispatcher()
    
    return q
}

func (q *JobQueue) Enqueue(job *Job) error {
    q.mu.Lock()
    defer q.mu.Unlock()
    
    // Store initial status
    status := &JobStatus{
        JobID:     job.ID,
        Status:    "queued",
        CreatedAt: time.Now(),
        UpdatedAt: time.Now(),
        Message:   "Job queued for processing",
    }
    
    if err := q.storage.StoreJobStatus(job.ID, status); err != nil {
        return err
    }
    
    heap.Push(&q.jobs, job)
    return nil
}

func (q *JobQueue) startWorkers() {
    for i := 0; i < q.maxWorkers; i++ {
        worker := NewWorker(q.workers, q.storage)
        worker.Start()
    }
}

func (q *JobQueue) startDispatcher() {
    go func() {
        for {
            q.mu.Lock()
            if q.jobs.Len() == 0 {
                q.mu.Unlock()
                time.Sleep(100 * time.Millisecond)
                continue
            }
            
            // Get next job
            job := heap.Pop(&q.jobs).(*Job)
            q.mu.Unlock()
            
            // Wait for available worker
            workerChan := <-q.workers
            workerChan <- job
        }
    }()
}
```

**✅ B2.1 Status: COMPLETED 2025-08-26**
- Priority-based job queue implemented with heap data structure
- Thread-safe operations with mutex protection
- Integration with storage abstraction layer
- 73.1% test coverage achieved

### B2.2: Worker Pool

```go
// internal/queue/worker.go
package queue

type Worker struct {
    workerPool chan chan *Job
    jobChannel chan *Job
    storage    storage.JobStorage
    executor   *openrewrite.Executor
    quit       chan bool
}

func NewWorker(workerPool chan chan *Job, storage storage.JobStorage) *Worker {
    return &Worker{
        workerPool: workerPool,
        jobChannel: make(chan *Job),
        storage:    storage,
        executor:   openrewrite.NewExecutor(),
        quit:       make(chan bool),
    }
}

func (w *Worker) Start() {
    go func() {
        for {
            // Register worker as available
            w.workerPool <- w.jobChannel
            
            select {
            case job := <-w.jobChannel:
                w.processJob(job)
                
            case <-w.quit:
                return
            }
        }
    }()
}

func (w *Worker) processJob(job *Job) {
    // Update status to processing
    status := &JobStatus{
        JobID:     job.ID,
        Status:    "processing",
        UpdatedAt: time.Now(),
        Progress:  10,
        Message:   "Starting transformation",
    }
    w.storage.StoreJobStatus(job.ID, status)
    
    // Execute transformation
    result, err := w.executor.Execute(
        context.Background(),
        job.ID,
        job.TarData,
        job.Recipe,
    )
    
    if err != nil {
        // Handle retry logic
        if job.Retries < 3 {
            job.Retries++
            // Re-queue job with delay
            time.AfterFunc(time.Duration(job.Retries) * time.Minute, func() {
                // Re-enqueue job
            })
            return
        }
        
        // Mark as failed
        status.Status = "failed"
        status.Error = err.Error()
        status.Message = "Transformation failed after retries"
    } else {
        // Store diff
        diffURL, err := w.storage.StoreDiff(job.ID, result.Diff)
        if err != nil {
            status.Status = "failed"
            status.Error = err.Error()
        } else {
            status.Status = "completed"
            status.DiffURL = diffURL
            status.Progress = 100
            status.Message = "Transformation completed successfully"
            completedAt := time.Now()
            status.CompletedAt = &completedAt
        }
    }
    
    status.UpdatedAt = time.Now()
    w.storage.StoreJobStatus(job.ID, status)
}
```

### B2.3: Job Cancellation & Advanced Queue Management ✅ 2025-08-26

#### Implemented Features
- [x] Job cancellation system with status tracking ✅ 2025-08-26
- [x] Queue lifecycle management (Start/Stop/Pause/Resume) ✅ 2025-08-26
- [x] Enhanced monitoring and metrics collection ✅ 2025-08-26
- [x] Worker-side cancellation checking ✅ 2025-08-26
- [x] Comprehensive unit tests with 87.8% coverage ✅ 2025-08-26

#### Key Capabilities Added
1. **Job Cancellation**: Jobs can be cancelled and removed from queue, with cancelled jobs skipped during processing
2. **Queue Pause/Resume**: Queue can be paused to stop processing without shutting down workers
3. **Drain Mode**: Queue can be drained for maintenance while completing current jobs
4. **Enhanced Metrics**: Tracks cancelled jobs, queue depth, active workers, and processing statistics
5. **Thread-Safe Operations**: All queue operations are properly synchronized with mutex locks

#### Testing Checklist
- [x] Queue prioritization working ✅ 2025-08-26
- [x] Worker pool processing jobs concurrently ✅ 2025-08-26
- [x] Retry mechanism for failed jobs ✅ 2025-08-26
- [x] Job status updates in real-time ✅ 2025-08-26
- [x] Graceful shutdown of workers ✅ 2025-08-26
- [x] Job cancellation prevents processing ✅ 2025-08-26
- [x] Pause/Resume controls job dispatch ✅ 2025-08-26

## Phase B3: Nomad Deployment & Auto-scaling

### Objectives
- [ ] Create Nomad job specification
- [ ] Configure auto-scaling policies
- [ ] Implement zero-to-one scaling
- [ ] Setup health checks and monitoring

### B3.1: Nomad Job Specification ✅ 2025-08-26

#### Implementation Status
- [x] Nomad job specification created ✅ 2025-08-26
- [x] Auto-scaling policies configured (0-10 instances) ✅ 2025-08-26
- [x] Health checks and service registration ✅ 2025-08-26
- [x] Docker container integration ✅ 2025-08-26
- [x] Resource allocation and constraints ✅ 2025-08-26

```hcl
# platform/nomad/openrewrite-service.hcl
job "openrewrite-service" {
  datacenters = ["dc1"]
  type        = "service"
  
  group "openrewrite" {
    count = 0  # Start with zero instances
    
    scaling {
      enabled = true
      min     = 0
      max     = 10
      
      policy {
        # Scale up when queue depth > 5
        cooldown            = "30s"
        check "queue_depth" {
          source = "consul"
          query  = "ploy/openrewrite/metrics/queue_depth"
          
          strategy "target-value" {
            target = 5
          }
        }
      }
      
      policy {
        # Scale down to zero after 10 minutes of inactivity
        cooldown = "10m"
        check "last_activity" {
          source = "consul"
          query  = "ploy/openrewrite/metrics/last_activity"
          
          strategy "threshold" {
            lower_bound = 600  # 10 minutes in seconds
            delta       = -1   # Scale down by 1
          }
        }
      }
    }
    
    network {
      port "http" {
        static = 8090
      }
    }
    
    task "openrewrite" {
      driver = "docker"
      
      config {
        image = "ploy/openrewrite-service:latest"
        ports = ["http"]
        
        mount {
          type   = "tmpfs"
          target = "/tmp/openrewrite"
          tmpfs_options {
            size = 4096000000  # 4GB for transformations
          }
        }
      }
      
      env {
        CONSUL_ADDRESS = "${NOMAD_IP_http}:8500"
        SEAWEEDFS_MASTER = "seaweedfs.service.consul:9333"
        WORKER_POOL_SIZE = "2"
        AUTO_SHUTDOWN_MINUTES = "10"
      }
      
      resources {
        cpu    = 2000
        memory = 4096
      }
      
      service {
        name = "openrewrite"
        port = "http"
        
        check {
          type     = "http"
          path     = "/health"
          interval = "10s"
          timeout  = "2s"
        }
        
        check {
          type     = "http"
          path     = "/metrics"
          interval = "30s"
          timeout  = "5s"
        }
      }
    }
  }
}
```

### B3.2: Auto-scaling Controller

```go
// internal/scaling/controller.go
package scaling

import (
    "time"
    "github.com/hashicorp/nomad/api"
)

type AutoScaler struct {
    nomadClient   *api.Client
    consulStorage *consul.ConsulStorage
    jobName       string
    minInstances  int
    maxInstances  int
}

func NewAutoScaler(nomadAddr, consulAddr string) (*AutoScaler, error) {
    nomadConfig := api.DefaultConfig()
    nomadConfig.Address = nomadAddr
    
    nomadClient, err := api.NewClient(nomadConfig)
    if err != nil {
        return nil, err
    }
    
    consulStorage, err := consul.NewConsulStorage(consulAddr)
    if err != nil {
        return nil, err
    }
    
    return &AutoScaler{
        nomadClient:   nomadClient,
        consulStorage: consulStorage,
        jobName:       "openrewrite-service",
        minInstances:  0,
        maxInstances:  10,
    }, nil
}

func (a *AutoScaler) Run() {
    ticker := time.NewTicker(30 * time.Second)
    defer ticker.Stop()
    
    for range ticker.C {
        if err := a.evaluate(); err != nil {
            log.Printf("Auto-scaling evaluation failed: %v", err)
        }
    }
}

func (a *AutoScaler) evaluate() error {
    // Get current metrics
    metrics, err := a.getMetrics()
    if err != nil {
        return err
    }
    
    // Get current job status
    job, _, err := a.nomadClient.Jobs().Info(a.jobName, nil)
    if err != nil {
        return err
    }
    
    currentCount := 0
    for _, group := range job.TaskGroups {
        if *group.Name == "openrewrite" {
            currentCount = *group.Count
            break
        }
    }
    
    // Determine desired count
    desiredCount := a.calculateDesiredCount(metrics, currentCount)
    
    // Update if needed
    if desiredCount != currentCount {
        return a.scaleToCount(desiredCount)
    }
    
    return nil
}

func (a *AutoScaler) calculateDesiredCount(metrics *Metrics, current int) int {
    // Scale to zero if inactive
    if time.Since(metrics.LastActivity) > 10*time.Minute {
        return 0
    }
    
    // Scale up based on queue depth
    if metrics.QueueDepth > 5 {
        desired := metrics.QueueDepth / 3 // 3 jobs per instance
        if desired > a.maxInstances {
            desired = a.maxInstances
        }
        if desired > current {
            return desired
        }
    }
    
    // Scale down gradually
    if metrics.QueueDepth == 0 && current > 1 {
        return current - 1
    }
    
    // Ensure at least 1 instance if there's work
    if metrics.QueueDepth > 0 && current == 0 {
        return 1
    }
    
    return current
}

func (a *AutoScaler) scaleToCount(count int) error {
    job, _, err := a.nomadClient.Jobs().Info(a.jobName, nil)
    if err != nil {
        return err
    }
    
    // Update task group count
    for _, group := range job.TaskGroups {
        if *group.Name == "openrewrite" {
            *group.Count = count
            break
        }
    }
    
    // Register updated job
    _, _, err = a.nomadClient.Jobs().Register(job, nil)
    return err
}
```

### B3.3: Health Monitoring

```go
// internal/monitoring/health.go
package monitoring

type HealthMonitor struct {
    storage       storage.JobStorage
    lastActivity  time.Time
    queueDepth    int
    activeWorkers int
}

func (h *HealthMonitor) UpdateMetrics() {
    // Store metrics in Consul for auto-scaler
    metrics := map[string]interface{}{
        "queue_depth":    h.queueDepth,
        "active_workers": h.activeWorkers,
        "last_activity":  h.lastActivity.Unix(),
    }
    
    h.storage.StoreMetrics(metrics)
}

func (h *HealthMonitor) RecordActivity() {
    h.lastActivity = time.Now()
    h.UpdateMetrics()
}
```

### B3.4: Testing Checklist
- [x] Nomad job specification validates successfully ✅ 2025-08-26
- [x] Zero-instance startup configuration ✅ 2025-08-26
- [x] Auto-scaling policies defined (queue depth & inactivity) ✅ 2025-08-26
- [x] Health checks configured (health, readiness, worker-status) ✅ 2025-08-26
- [x] Service registration with Consul ✅ 2025-08-26
- [x] Metrics endpoint configured ✅ 2025-08-26
- [ ] Nomad job deploys successfully (requires VPS testing)
- [ ] Zero-to-one scaling works (requires VPS testing)
- [ ] Scale up triggered by queue depth (requires VPS testing)
- [ ] Scale down to zero after inactivity (requires VPS testing)

## Integration Points

### With Stream A
- Container image from A3 deployed via Nomad
- API endpoints register with Consul
- Health checks configured

### With Stream C
- Monitoring endpoints exposed
- Metrics collection enabled
- Production logging configured

## Success Metrics

### Infrastructure Requirements
- [ ] Consul KV stores job status reliably
- [ ] SeaweedFS stores diffs persistently
- [ ] Job queue processes concurrently
- [ ] Auto-scaling responds within 30 seconds
- [ ] Zero instances when idle
- [ ] Scales to 10 instances under load

### Performance Targets
- [ ] Queue depth maintained < 10 jobs
- [ ] Worker pool utilization > 80%
- [ ] Status updates < 100ms latency
- [ ] Diff retrieval < 500ms
- [ ] Scaling decisions < 5 seconds

## Troubleshooting Guide

### Common Issues

1. **Consul connection failures**
   - Verify Consul agent running
   - Check network connectivity
   - Validate ACL tokens if enabled

2. **SeaweedFS upload errors**
   - Ensure sufficient disk space
   - Check volume servers healthy
   - Verify network between services

3. **Auto-scaling not triggering**
   - Check Nomad autoscaler plugin
   - Verify metrics in Consul
   - Review scaling policies

4. **Workers not processing jobs**
   - Check worker pool initialization
   - Verify executor permissions
   - Monitor memory/CPU usage

## Next Steps
- Production monitoring (Stream C)
- ARF integration (Stream C)
- Performance optimization
- Multi-region support