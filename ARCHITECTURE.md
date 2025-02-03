# Mycelium Architecture

## Overview

Mycelium is a distributed data store system with role-based access control, designed to support a network of validator nodes (which participate in consensus) and miner nodes (which process work). Each validator node runs a Consensus Manager component, with leadership determined through weighted Raft election based on validator tiers and performance metrics.

## System Architecture Diagram

```mermaid
graph TB
    subgraph "Validator 1 (Tier 0)"
        V1[Validator Process]
        CM1[Consensus Manager]
        WR1[Weighted Raft Node]
        EO1[Emergency Override]
    end

    subgraph "Validator 2 (Tier 1)"
        V2[Validator Process]
        CM2[Consensus Manager]
        WR2[Weighted Raft Node]
        EO2[Emergency Override]
    end

    subgraph "Validator 3 (Tier 2)"
        V3[Validator Process]
        CM3[Consensus Manager]
        WR3[Weighted Raft Node]
        EO3[Emergency Override]
    end

    subgraph Miners
        M1[Miner 1]
        M2[Miner 2]
    end

    subgraph "Data Store"
        S1[Shard 1]
        S2[Shard 2]
        S3[Shard 3]
    end

    subgraph "Communication Layer"
        NATS[NATS PubSub]
        GRPC[gRPC Mesh]
    end

    V1 --> CM1
    CM1 --> WR1
    V1 --> EO1
    EO1 --> WR1

    V2 --> CM2
    CM2 --> WR2
    V2 --> EO2
    EO2 --> WR2

    V3 --> CM3
    CM3 --> WR3
    V3 --> EO3
    EO3 --> WR3

    WR1 -->|Leader Write| S1
    WR2 -->|Leader Write| S2
    WR3 -->|Leader Write| S3
    
    M1 -->|Read| S1
    M1 -->|Read| S2
    M2 -->|Read| S2
    M2 -->|Read| S3

    WR1 <-->|Weighted Consensus| WR2
    WR2 <-->|Weighted Consensus| WR3
    WR3 <-->|Weighted Consensus| WR1

    EO1 -.->|Emergency Control| WR2
    EO1 -.->|Emergency Control| WR3

    CM1 <-->|Mesh| GRPC
    CM2 <-->|Mesh| GRPC
    CM3 <-->|Mesh| GRPC

    M1 <-->|Subscribe| NATS
    M2 <-->|Subscribe| NATS

    CM1 -->|Distribute Work| NATS
    CM2 -->|Distribute Work| NATS
    CM3 -->|Distribute Work| NATS
```

## Work Distribution Flow

```mermaid
sequenceDiagram
    participant V as Validator
    participant CM as Local Consensus Manager
    participant WR as Weighted Raft Group
    participant NATS as NATS PubSub
    participant M as Miner

    Note over V,WR: If local CM is leader
    V->>CM: Submit Work Item
    CM->>WR: Propose Distribution
    WR->>WR: Weight-based Voting
    WR->>CM: Confirm Leadership
    CM->>NATS: Distribute Work
    NATS->>M: Work Request
    
    M->>M: Process Work
    M->>V: Submit Result
    V->>CM: Score Result
    CM->>WR: Propose Score
    WR->>WR: Weighted Consensus
    WR->>CM: Confirm Update
    CM->>NATS: Broadcast Result
```

## Consensus Architecture

```mermaid
graph LR
    subgraph "Validator Node"
        VP[Validator Process]
        CM[Consensus Manager]
        WD[Work Distributor]
        VS[Validator Scoring]
        WRC[Weighted Raft Client]
        EO[Emergency Override]
    end

    subgraph "Consensus Layer"
        WRN[Weighted Raft Node]
        WVS[Voter State]
        CS[Consensus State]
        LOG[Raft Log]
    end

    subgraph "Storage Layer"
        DB[PostgreSQL]
        PART[Partitions]
        REP[Replicas]
    end

    VP -->|Submit| CM
    CM -->|Coordinate| WD
    CM -->|Process| VS
    CM -->|Propose| WRC
    WRC -->|Weight-based Vote| WRN
    EO -->|Emergency Control| WRN
    WRN -->|Update| WVS
    WRN -->|Update| CS
    CS -->|Commit| DB
    DB -->|Shard| PART
    PART -->|Replicate| REP

    classDef leader fill:#f96
    class CM,WRN leader
```

## Core Components

### 1. Node Types

#### Validators
- Primary nodes with read/write access
- Each runs a Consensus Manager component
- Participates in Raft consensus groups
- Leader election per shard
- Must sign all data modifications
- Responsible for work distribution and scoring

#### Miners
- Read-only access to data
- Process work assigned by validator leaders
- Submit results for validator scoring
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

### 4. Leadership Security Model

#### Trusted Validator Tiers
```mermaid
graph TB
    subgraph "Tier 0 (Bootstrap)"
        T0[Dev Team Validators]
        KEY[Root Keys]
    end

    subgraph "Tier 1 (Vetted)"
        T1A[Trusted Partner 1]
        T1B[Trusted Partner 2]
        T1C[Trusted Partner 3]
    end

    subgraph "Tier 2 (Public)"
        T2A[Public Validator 1]
        T2B[Public Validator 2]
        T2C[Public Validator 3]
    end

    T0 -->|Sign & Authorize| T1A
    T0 -->|Sign & Authorize| T1B
    T0 -->|Sign & Authorize| T1C

    T1A -->|Vouch| T2A
    T1B -->|Vouch| T2B
    T1C -->|Vouch| T2C

    KEY -->|Root Trust| T0
```

#### Emergency Override System
```mermaid
graph TB
    subgraph "Emergency Actions"
        FS[Force Step Down]
        BV[Block Validator]
        UV[Unblock Validator]
        SS[Force State Sync]
    end

    subgraph "Security Controls"
        T0[Tier 0 Authorization]
        LOG[Action Logging]
        SIG[Multi-Signatures]
        AUDIT[Audit Trail]
    end

    subgraph "Recovery Process"
        DETECT[Detect Issue]
        INITIATE[Initiate Override]
        APPROVE[Multi-Sig Approval]
        EXECUTE[Execute Action]
        VERIFY[Verify State]
    end

    T0 -->|Authorize| FS
    T0 -->|Authorize| BV
    T0 -->|Authorize| UV
    T0 -->|Authorize| SS

    DETECT -->|Trigger| INITIATE
    INITIATE -->|Require| T0
    INITIATE -->|Record| LOG
    T0 -->|Collect| SIG
    SIG -->|Enable| APPROVE
    APPROVE -->|Allow| EXECUTE
    EXECUTE -->|Generate| AUDIT
    EXECUTE -->|Confirm| VERIFY
```

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