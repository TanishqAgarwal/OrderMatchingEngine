# Order Matching Engine

Kept the file structure according to what I read the industry standards are. Also saw that in go test files are written parallel to the logical entity being tested, so I did'nt make a separate tests folder.

## Overview

This project implements a limit order book matching engine designed for low latency and high throughput. It supports standard trading operations including Limit Orders, Market Orders, and Cancellations, all exposed via a REST API.

**Key Features:**
*   **High Performance:** ~700,000+ orders/second matching engine throughput.
*   **Low Latency:** Average engine latency < 2 microseconds.
*   **Concurrency:** Thread-safe execution with granular symbol-level locking.
*   **Correctness:** Strict price-time priority and "All-or-None" liquidity checks for Market Orders.
*   **Observability:** Real-time metrics with accurate P50, P99, and P999 latency histograms (microsecond precision).

## Quick Start

### Build and Run
Prerequisites: Go 1.25+

1.  **Clone the repository:**
    ```bash
    git clone https://github.com/TanishqAgarwal/OrderMatchingEngine.git
    cd OrderMatchingEngine
    ```

2.  **Run the server:**
    ```bash
    go run cmd/server/main.go
    ```
    The server will start on `http://localhost:8080`.

### Run Tests & Benchmarks

*   **Run Unit Tests:**
    ```bash
    go test -v ./internal/matching
    ```

*   **Run Race Detector (Concurrency Check):**
    ```bash
    go test -v -race ./...
    ```

*   **Run Performance Benchmarks:**
    ```bash
    go test -bench=. ./internal/matching
    ```

## Architecture & Approach

The system uses a **Red-Black Tree** to store order books, ensuring `O(log N)` time complexity for inserting, removing, and matching orders. This is superior to a simple slice (O(N) insertion) for maintaining a sorted price-time priority queue.

**Concurrency Model:**
*   **OrderBook Level Locking:** Instead of a single global lock, each Order Book (Symbol) has its own `sync.RWMutex`. This allows orders for different symbols (e.g., BTC vs. ETH) to be processed in parallel on different CPU cores.
*   **Global Lookup:** A thread-safe `sync.Map` stores all active orders for `O(1)` access during cancellation or status checks.
*   **Lock-Free Metrics:** Latency tracking uses a high-performance, lock-free histogram (atomic counters) to calculate accurate percentiles without impacting trading throughput.

## Performance Results

Benchmarks run on an Apple M1 Pro (8-core):

| Metric | Result | Target | Status |
| :--- | :--- | :--- | :--- |
| **Throughput** | **~705,716 ops/sec** | 30,000 ops/sec (required)
| **Latency (Avg)** | **1.4 microseconds** | < 10 ms (required)

## API Endpoints

*   `POST /api/v1/orders` - Submit a new Limit or Market order.
*   `DELETE /api/v1/orders/{id}` - Cancel an active order.
*   `GET /api/v1/orders/{id}` - Get order status.
*   `GET /api/v1/orderbook/{symbol}` - Get current book depth.
*   `GET /health` - Service health check.
*   `GET /metrics` - Real-time system metrics.

## Future Improvements

*   **Symbol Whitelist:** Currently, the engine accepts any string as a symbol. A production system should validate against a predefined list (e.g., allow "BTC-USD", reject "XYZ-FAKE") to prevent spam.
*   **Persistence:** The order book is currently in-memory. Adding a database integration would ensure data survives server restarts.
