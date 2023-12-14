# Service Infrastructure and Pipeline

This robust service infrastructure and pipeline, built with Golang, is designed to handle real-time data processing and distribution for cryptocurrency markets and local orderbook over Nats clusters. This README provides a comprehensive breakdown of the system's architecture, components, and data flow.

## Table of Contents

- [Service Infrastructure and Pipeline](#service-infrastructure-and-pipeline)
  - [Table of Contents](#table-of-contents)
  - [Introduction](#introduction)
  - [System Architecture](#system-architecture)
    - [Service Initialization](#service-initialization)
    - [Core Components](#core-components)
    - [Data Flow and Processing](#data-flow-and-processing)
  - [High Availability and Resilience](#high-availability-and-resilience)
  - [etcd and Service Discovery](#etcd-and-service-discovery)
  - [Scalability](#scalability)
    - [Kubernetes HPA (Horizontal Pod Autoscaling)](#kubernetes-hpa-horizontal-pod-autoscaling)
  - [External Resources](#external-resources)
  - [Conclusion](#conclusion)

## Introduction

In the fast-paced world of cryptocurrency, real-time data processing and distribution are crucial. Our infrastructure is meticulously designed to ensure data integrity, availability, and resilience, making it a reliable backbone for any crypto trading platform.

## System Architecture

### Service Initialization

Services are initialized using flags, which act as service identifiers. These flags simplify the process of starting, stopping, or modifying specific services:

- `orderbook`: Manages order book data.
- `trades`: Handles trade data.
- `tickers`: Manages ticker data.
- `markets`: Handles overall market data.
- `--port`: An optional flag to specify a port for local testing.

### Core Components

- **Redis**: A high-performance in-memory database system. In this infrastructure, Redis plays a crucial role in caching and providing fast access to data, especially for frequently accessed datasets like recent trades.

- **Nats Cluster**: A [distributed messaging system](https://nats.io/) with three nodes. It's the backbone of our inter-service communication, ensuring that data is consistently distributed across all services.

- **WebSocket**: This component is responsible for maintaining real-time connections, crucial for streaming live data updates to end-users.

These components are fortified with self-recovery mechanisms and utilize incremental backoff strategies to manage failures, ensuring uninterrupted system operation even in unpredictable scenarios.

![System Architecture Diagram](https://ummcsnegloedxcrwlucz.supabase.co/storage/v1/object/public/chatgpt-diagrams/2023-09-14/d00cb4fc-3c9f-4c29-a364-07370f898477.png)

### Data Flow and Processing

1. **Orderbook Service**:
   - From Assets service get two different list of pairs. First one is the most important ones and others. 
   - Start with the first important ones and continue to process other pairs with same steps from beginning.
   - Initially, it fetches a snapshot of the market, gathering up to 5000 rows of depth for each trading pair.
   - It then establishes WebSocket subscriptions with Binance, receiving live updates for each trading pair.
   - Using Ring Buffers, the service efficiently manages and updates local order books.
   - Another routine sorts and maintains the order of asks and bids.
   - Data is then filtered based on settings from an external API, ensuring that anomalies (like orders beyond a certain spread percentage) are removed.
   - Finally, processed order books are dispatched to the Nats cluster for distribution.

2. **Tickers Service**:
   - This service focuses on ticker data, subscribing to Binance for live updates.
   - Data is processed and then dispatched to the Nats cluster under specific topics.

3. **Trades Service**:
   - Manages live trade data.
   - <span style="text-decoration: overline; color: red;">The most recent trades (up to 10) for each pair are cached in Redis. This ensures that users receive immediate data upon subscription.</span> Deprecated, planning to add this to stream-worker's responsibility
   - Live trade updates are also dispatched to the Nats cluster.

4. **Markets Service**:
   - This service aggregates market data for all trading pairs.
   - An initial snapshot is created and stored in Redis, which is then kept updated in real-time.
   - Live market updates are dispatched to the Nats cluster.

## High Availability and Resilience

This system employs Kubernetes etcd for leader election, ensuring that even if a primary service instance fails, a secondary instance can immediately take over, ensuring uninterrupted service. This leader-follower model, combined with [Kubernetes probes](https://kubernetes.io/docs/tasks/configure-pod-container/configure-liveness-readiness-startup-probes/), ensures that our services are not only fault-tolerant but can also self-recover from failures.

## etcd and Service Discovery

- **etcd**: A distributed key-value store used for configuration management and service discovery. It ensures consistency across distributed systems using the Raft consensus algorithm.
  
- **Service Discovery**: Allows services in a distributed system to dynamically find and communicate with each other. It adapts to changes in the system, such as service failures or scaling events.

  - **Patterns**: Includes client-side discovery, server-side discovery, self-registration, and third-party registration.

  - **Benefits**: Enables dynamic infrastructure, load balancing, and high availability in distributed systems.

## Scalability

The architecture is designed to scale horizontally. As the demand grows, additional instances of services can be spawned to handle the increased load. Load balancers distribute incoming requests to ensure no single service instance is overwhelmed. This design ensures that the system can handle a surge in users or data volume without compromising performance.

### Kubernetes HPA (Horizontal Pod Autoscaling)

Kubernetes HPA allows, services to automatically scale the number of pods in a deployment or replica set based on observed metrics like CPU utilization or, in our case, custom metrics provided through Prometheus. 

- **Integration with Prometheus**: I have integrated Prometheus to monitor the services and send metrics to Kubernetes. These metrics are then used by the HPA to make scaling decisions. For instance, if a particular service is experiencing a surge in traffic, Prometheus metrics will reflect this, and the HPA will automatically scale up the number of pods for that service.

- **Individual Service Scaling**: Each service in architecture can be scaled individually based on its own set of metrics. This ensures that resources are used efficiently and that each service is scaled appropriately based on its own demand.

## External Resources

- [Kubernetes Probes Documentation](https://kubernetes.io/docs/tasks/configure-pod-container/configure-liveness-readiness-startup-probes/)
- [Nats Official Documentation](https://nats.io/)
- [Kubernetes HPA Documentation](https://kubernetes.io/docs/tasks/run-application/horizontal-pod-autoscale/)

## Conclusion

The service infrastructure and pipeline stand as a testament to robust engineering and meticulous design. By leveraging cutting-edge technologies and best practices, I've built a system that's not only reliable but also scalable, ready to meet the demands of the ever-evolving world of cryptocurrency trading.

> @denizumutdereli
