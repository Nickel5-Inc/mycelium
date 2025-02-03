# Mycelium Architecture

## Overview

Mycelium is a distributed data store system with role-based access control, designed to support a network of validator and miner nodes. The system implements sharding, replication, and secure data management through PostgreSQL's native features, with a lightweight consensus mechanism for work distribution and validation.

## System Architecture Diagram

```mermaid
graph TB
    subgraph Validators
        V1[Validator 1]
        V2[Validator 2]
        V3[Validator 3]
    end

    subgraph Miners
        M1[Miner 1]
        M2[Miner 2]
    end

    subgraph Data Store
        S1[Shard 1]
        S2[Shard 2]
        S3[Shard 3]
    end

    subgraph Consensus Layer
        R1[Raft Group 1]
        R2[Raft Group 2]
        R3[Raft Group 3]
        CM[Consensus Manager]
    end

    subgraph Communication Layer
        NATS[NATS PubSub]
        GRPC[gRPC Mesh]
    end

    V1 -->|Write| S1
    V2 -->|Write| S2
    V3 -->|Write| S3
    
    M1 -->|Read| S1
    M1 -->|Read| S2
    M2 -->|Read| S2
    M2 -->|Read| S3

    V1 -->|Participate| R1
    V2 -->|Participate| R2
    V3 -->|Participate| R3

    V1 <-->|Mesh| GRPC
    V2 <-->|Mesh| GRPC
    V3 <-->|Mesh| GRPC

    M1 <-->|Subscribe| NATS
    M2 <-->|Subscribe| NATS

    CM -->|Coordinate| R1
    CM -->|Coordinate| R2
    CM -->|Coordinate| R3

    CM -->|Distribute Work| NATS
    CM <-->|Collect Scores| GRPC
```

## Work Distribution Flow

```mermaid
sequenceDiagram
    participant V as Validator
    participant CM as Consensus Manager
    participant NATS as NATS PubSub
    participant M as Miner
    participant R as Raft Group

    V->>CM: Submit Work Item
    CM->>NATS: Distribute Work
    NATS->>M: Work Request
    M->>M: Process Work
    M->>V: Submit Result
    V->>CM: Score Result
    CM->>CM: Calculate Consensus
    CM->>R: Propose Update
    R->>R: Reach Agreement
    R->>CM: Confirm Update
    CM->>NATS: Broadcast Result
```

## Consensus Architecture

```mermaid
graph LR
    subgraph Validator Node
        WD[Work Distributor]
        VS[Validator Scoring]
        RC[Raft Client]
    end

    subgraph Consensus Layer
        RN[Raft Node]
        CS[Consensus State]
        LOG[Raft Log]
    end

    subgraph Storage Layer
        DB[PostgreSQL]
        PART[Partitions]
        REP[Replicas]
    end

    WD -->|Submit| VS
    VS -->|Propose| RC
    RC -->|Log| RN
    RN -->|Update| CS
    CS -->|Commit| DB
    DB -->|Shard| PART
    PART -->|Replicate| REP
```

## Core Components

### 1. Node Types

#### Validators
- Primary nodes with read/write access
- Responsible for data validation and shard management
- Can initialize and assign shards
- Must sign all data modifications

#### Miners (Future)
- Read-only access to data
- Host and serve data replicas
- Cannot modify stored data
- Will participate in data availability and serving

### 2. Data Architecture

#### Sharding System
- Uses PostgreSQL's native table partitioning
- List partitioning based on shard_id
- Each shard is a separate partition
- Automatic query routing through partition pruning

#### Replication
- Leverages PostgreSQL's Foreign Data Wrapper (FDW)
- Cross-node replication with role-based permissions
- Asynchronous replication with change tracking
- Configurable replica count per shard

### 3. Security Model

#### Role-Based Access Control (RBAC)
- Validator role: Full read/write access
- Miner role: Read-only access
- Database-level permission enforcement
- Trigger-based validation of modifications

#### Data Integrity
- Required signatures for all modifications
- Public key infrastructure for node identification
- Cryptographic verification of changes
- Audit trail of modifications

## Detailed Component Specifications

### 1. Database Schema

#### Node Metadata
```sql
CREATE TABLE node_metadata (
    node_id TEXT PRIMARY KEY,
    version TEXT NOT NULL,
    ip TEXT NOT NULL,
    port INTEGER NOT NULL,
    node_type TEXT NOT NULL,
    public_key TEXT NOT NULL,
    last_seen TIMESTAMP WITH TIME ZONE,
    capabilities JSONB,
    shard_ranges JSONB,
    is_active BOOLEAN
)
```

#### Sharded Data
```sql
CREATE TABLE data_template (
    key TEXT NOT NULL,
    value JSONB NOT NULL,
    version INTEGER NOT NULL,
    created_by TEXT NOT NULL,
    signature TEXT NOT NULL,
    shard_id TEXT NOT NULL,
    PRIMARY KEY (shard_id, key)
) PARTITION BY LIST (shard_id)
```

### 2. Shard Management

#### Initialization Process
1. Validator requests shard creation
2. System generates unique shard ID
3. Creates partition in database
4. Assigns initial node responsibility
5. Updates metadata and routing information

#### Replica Assignment
1. Validator selects target node for replica
2. Sets up foreign data wrapper connection
3. Creates appropriate user mappings
4. Grants necessary permissions
5. Initiates initial data synchronization

### 3. Synchronization System

#### Change Tracking
- Every modification is logged in sync_changes table
- Includes operation type, timestamp, and node ID
- Tracks sync status and attempts
- Handles failure recovery

#### Sync Process
1. Source node identifies unsynced changes
2. Orders changes by timestamp
3. Applies changes to replica nodes
4. Updates sync status
5. Verifies data consistency

### 4. Protocol Version Management

#### Version Control
- Tracks protocol versions and compatibility
- Manages feature flags and capabilities
- Ensures network-wide version compatibility
- Handles protocol upgrades

#### Feature Negotiation
- Dynamic feature detection
- Capability advertisement
- Backwards compatibility handling
- Gradual feature rollout support

## Implementation Details

### 1. Core Managers

#### ShardManager
- Handles shard lifecycle
- Manages replica assignments
- Coordinates synchronization
- Monitors shard health

#### MetadataManager
- Tracks node information
- Manages protocol versions
- Handles feature negotiation
- Maintains network state

### 2. Security Implementation

#### Access Control
- Database-level RBAC
- Trigger-based validation
- Signature verification
- Node authentication

#### Data Protection
- Cryptographic signatures
- Secure connections
- Audit logging
- Permission enforcement

### 3. Scalability Features

#### Horizontal Scaling
- Dynamic shard creation
- Automatic load balancing
- Replica distribution
- Cross-node queries

#### Performance Optimization
- Partition pruning
- Index management
- Query optimization
- Connection pooling

## Future Considerations

### 1. Miner Integration
- Implementation of miner node functionality
- Data serving optimization
- Read-only access patterns
- Caching strategies

### 2. Advanced Features
- Automatic shard rebalancing
- Dynamic replica adjustment
- Advanced monitoring
- Performance analytics

### 3. Network Expansion
- Cross-region support
- Latency optimization
- Geographic distribution
- Network topology management

## Operational Aspects

### 1. Monitoring
- Shard health checks
- Sync status monitoring
- Node availability tracking
- Performance metrics

### 2. Maintenance
- Schema migrations
- Version upgrades
- Data cleanup
- Index optimization

### 3. Disaster Recovery
- Backup strategies
- Recovery procedures
- Data consistency checks
- Failover handling

## Development Guidelines

### 1. Code Organization
- Clear package structure
- Interface-based design
- Error handling patterns
- Testing requirements

### 2. Best Practices
- Transaction management
- Error propagation
- Logging standards
- Documentation requirements

## Conclusion

This architecture provides a robust foundation for a distributed data store with strong security guarantees and clear role separation. The system is designed to scale horizontally while maintaining data integrity and providing efficient access patterns for both validator and miner nodes. 