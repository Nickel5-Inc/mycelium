package database

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5"
)

// Schema version for database migrations
const CurrentSchemaVersion = 1

// Schema contains all table definitions and migrations
var Schema = []string{
	// Enable required extensions
	`CREATE EXTENSION IF NOT EXISTS postgres_fdw`,
	`CREATE EXTENSION IF NOT EXISTS pgcrypto`, // For encryption functions

	// Create roles
	`DO $$
	BEGIN
		IF NOT EXISTS (SELECT FROM pg_roles WHERE rolname = 'validator') THEN
			CREATE ROLE validator;
		END IF;
		IF NOT EXISTS (SELECT FROM pg_roles WHERE rolname = 'miner') THEN
			CREATE ROLE miner;
		END IF;
	END
	$$`,

	// Node metadata table with role information
	`CREATE TABLE IF NOT EXISTS node_metadata (
		node_id TEXT PRIMARY KEY,
		version TEXT NOT NULL,
		ip TEXT NOT NULL,
		port INTEGER NOT NULL,
		node_type TEXT NOT NULL CHECK (node_type IN ('validator', 'miner')),
		public_key TEXT NOT NULL,
		last_seen TIMESTAMP WITH TIME ZONE NOT NULL,
		capabilities JSONB NOT NULL DEFAULT '{}'::jsonb,
		shard_ranges JSONB NOT NULL DEFAULT '[]'::jsonb,
		is_active BOOLEAN NOT NULL DEFAULT true,
		created_at TIMESTAMP WITH TIME ZONE NOT NULL DEFAULT NOW(),
		updated_at TIMESTAMP WITH TIME ZONE NOT NULL DEFAULT NOW()
	)`,

	// Emergency action logs table for security audit
	`CREATE TABLE IF NOT EXISTS emergency_action_logs (
		id BIGSERIAL PRIMARY KEY,
		timestamp TIMESTAMP WITH TIME ZONE NOT NULL,
		initiator_id TEXT NOT NULL REFERENCES node_metadata(node_id),
		target_id TEXT NOT NULL REFERENCES node_metadata(node_id),
		action TEXT NOT NULL,
		raft_state TEXT NOT NULL,
		signatures JSONB NOT NULL,
		created_at TIMESTAMP WITH TIME ZONE NOT NULL DEFAULT NOW(),
		CONSTRAINT valid_action CHECK (action IN ('force_step_down', 'block_validator', 'unblock_validator', 'force_sync'))
	)`,

	// Add index for querying emergency logs
	`CREATE INDEX IF NOT EXISTS idx_emergency_logs_timestamp ON emergency_action_logs(timestamp)`,
	`CREATE INDEX IF NOT EXISTS idx_emergency_logs_initiator ON emergency_action_logs(initiator_id)`,
	`CREATE INDEX IF NOT EXISTS idx_emergency_logs_target ON emergency_action_logs(target_id)`,

	// Only validators can write emergency logs
	`GRANT SELECT, INSERT ON emergency_action_logs TO validator`,
	`GRANT SELECT ON emergency_action_logs TO miner`,

	// Sharded data table template with security
	`CREATE TABLE IF NOT EXISTS data_template (
		key TEXT NOT NULL,
		value JSONB NOT NULL,
		version INTEGER NOT NULL,
		created_by TEXT NOT NULL REFERENCES node_metadata(node_id),
		signature TEXT NOT NULL,  -- Validator's signature of the data
		created_at TIMESTAMP WITH TIME ZONE NOT NULL DEFAULT NOW(),
		updated_at TIMESTAMP WITH TIME ZONE NOT NULL DEFAULT NOW(),
		shard_id TEXT NOT NULL,
		PRIMARY KEY (shard_id, key)
	) PARTITION BY LIST (shard_id)`,

	// Access control function
	`CREATE OR REPLACE FUNCTION check_validator_access() RETURNS TRIGGER AS $$
	BEGIN
		IF NOT EXISTS (
			SELECT 1 FROM node_metadata 
			WHERE node_id = current_user 
			AND node_type = 'validator' 
			AND is_active = true
		) THEN
			RAISE EXCEPTION 'Only active validators can modify data';
		END IF;
		RETURN NEW;
	END;
	$$ LANGUAGE plpgsql`,

	// Apply trigger to data template
	`DROP TRIGGER IF EXISTS ensure_validator_access ON data_template`,
	`CREATE TRIGGER ensure_validator_access
		BEFORE INSERT OR UPDATE OR DELETE ON data_template
		FOR EACH ROW EXECUTE FUNCTION check_validator_access()`,

	// Grant appropriate permissions
	`GRANT SELECT ON ALL TABLES IN SCHEMA public TO miner`,
	`GRANT SELECT, INSERT, UPDATE ON ALL TABLES IN SCHEMA public TO validator`,
	`ALTER DEFAULT PRIVILEGES IN SCHEMA public GRANT SELECT ON TABLES TO miner`,
	`ALTER DEFAULT PRIVILEGES IN SCHEMA public GRANT SELECT, INSERT, UPDATE ON TABLES TO validator`,

	// Protocol version tracking
	`CREATE TABLE IF NOT EXISTS protocol_versions (
		id SERIAL PRIMARY KEY,
		version TEXT NOT NULL,
		min_compatible_version TEXT NOT NULL,
		features JSONB NOT NULL,
		is_active BOOLEAN NOT NULL DEFAULT true,
		activation_time TIMESTAMP WITH TIME ZONE NOT NULL,
		created_at TIMESTAMP WITH TIME ZONE NOT NULL DEFAULT NOW()
	)`,

	// Shard allocation tracking with partition management
	`CREATE TABLE IF NOT EXISTS shard_allocations (
		shard_id TEXT PRIMARY KEY,
		node_id TEXT NOT NULL REFERENCES node_metadata(node_id),
		is_primary BOOLEAN NOT NULL DEFAULT false,
		replica_count INTEGER NOT NULL DEFAULT 0,
		last_sync TIMESTAMP WITH TIME ZONE,
		status TEXT NOT NULL DEFAULT 'pending',
		metadata JSONB NOT NULL DEFAULT '{}'::jsonb,
		partition_name TEXT UNIQUE NOT NULL,
		created_at TIMESTAMP WITH TIME ZONE NOT NULL DEFAULT NOW(),
		updated_at TIMESTAMP WITH TIME ZONE NOT NULL DEFAULT NOW()
	)`,

	// Shard sync tracking
	`CREATE TABLE IF NOT EXISTS shard_sync_status (
		id SERIAL PRIMARY KEY,
		shard_id TEXT NOT NULL REFERENCES shard_allocations(shard_id),
		source_node TEXT NOT NULL REFERENCES node_metadata(node_id),
		target_node TEXT NOT NULL REFERENCES node_metadata(node_id),
		sync_started TIMESTAMP WITH TIME ZONE NOT NULL,
		sync_completed TIMESTAMP WITH TIME ZONE,
		status TEXT NOT NULL DEFAULT 'pending',
		error_message TEXT,
		metadata JSONB NOT NULL DEFAULT '{}'::jsonb
	)`,

	// Change tracking for synchronization
	`CREATE TABLE IF NOT EXISTS sync_changes (
		id BIGSERIAL PRIMARY KEY,
		table_name TEXT NOT NULL,
		operation TEXT NOT NULL,
		record_id TEXT NOT NULL,
		data JSONB NOT NULL,
		timestamp TIMESTAMP WITH TIME ZONE NOT NULL DEFAULT NOW(),
		node_id TEXT NOT NULL,
		shard_id TEXT,
		is_synced BOOLEAN NOT NULL DEFAULT false,
		sync_attempts INTEGER NOT NULL DEFAULT 0,
		last_sync_attempt TIMESTAMP WITH TIME ZONE,
		error_message TEXT
	)`,

	// Indexes
	`CREATE INDEX IF NOT EXISTS idx_node_metadata_type ON node_metadata(node_type)`,
	`CREATE INDEX IF NOT EXISTS idx_data_template_created ON data_template(created_by)`,
	`CREATE INDEX IF NOT EXISTS idx_node_metadata_active ON node_metadata(is_active)`,
	`CREATE INDEX IF NOT EXISTS idx_shard_allocations_node ON shard_allocations(node_id)`,
	`CREATE INDEX IF NOT EXISTS idx_sync_changes_unsynced ON sync_changes(is_synced, timestamp) WHERE NOT is_synced`,
	`CREATE INDEX IF NOT EXISTS idx_protocol_versions_active ON protocol_versions(is_active)`,

	// Functions for shard management with security
	`CREATE OR REPLACE FUNCTION create_shard_partition(
		p_shard_id TEXT,
		p_partition_name TEXT
	) RETURNS void AS $$
	BEGIN
		-- Verify caller is a validator
		IF NOT EXISTS (
			SELECT 1 FROM node_metadata 
			WHERE node_id = current_user 
			AND node_type = 'validator' 
			AND is_active = true
		) THEN
			RAISE EXCEPTION 'Only active validators can create shards';
		END IF;

		EXECUTE format(
			'CREATE TABLE IF NOT EXISTS %I PARTITION OF data_template 
			FOR VALUES IN (%L)',
			p_partition_name,
			p_shard_id
		);
	END;
	$$ LANGUAGE plpgsql SECURITY DEFINER`,

	`CREATE OR REPLACE FUNCTION setup_foreign_shard(
		p_shard_id TEXT,
		p_server_name TEXT,
		p_remote_schema TEXT,
		p_partition_name TEXT,
		p_node_type TEXT
	) RETURNS void AS $$
	BEGIN
		-- Create foreign server if it doesn't exist
		EXECUTE format(
			'CREATE SERVER IF NOT EXISTS %I
			FOREIGN DATA WRAPPER postgres_fdw
			OPTIONS (dbname %L)',
			p_server_name,
			current_database()
		);
		
		-- Create user mapping with appropriate credentials based on node type
		EXECUTE format(
			'CREATE USER MAPPING IF NOT EXISTS FOR %I SERVER %I 
			OPTIONS (user %L)',
			current_user,
			p_server_name,
			CASE 
				WHEN p_node_type = 'validator' THEN 'validator'
				ELSE 'miner'
			END
		);
		
		-- Create foreign table with appropriate permissions
		EXECUTE format(
			'CREATE FOREIGN TABLE %I (
				key TEXT NOT NULL,
				value JSONB NOT NULL,
				version INTEGER NOT NULL,
				created_by TEXT NOT NULL,
				signature TEXT NOT NULL,
				created_at TIMESTAMP WITH TIME ZONE NOT NULL,
				updated_at TIMESTAMP WITH TIME ZONE NOT NULL,
				shard_id TEXT NOT NULL
			) SERVER %I OPTIONS (
				schema_name %L,
				table_name %L
			)',
			p_partition_name,
			p_server_name,
			p_remote_schema,
			p_partition_name
		);
		
		-- Attach as partition
		EXECUTE format(
			'ALTER TABLE data_template ATTACH PARTITION %I 
			FOR VALUES IN (%L)',
			p_partition_name,
			p_shard_id
		);

		-- Grant appropriate permissions
		IF p_node_type = 'validator' THEN
			EXECUTE format(
				'GRANT SELECT, INSERT, UPDATE ON %I TO validator',
				p_partition_name
			);
		ELSE
			EXECUTE format(
				'GRANT SELECT ON %I TO miner',
				p_partition_name
			);
		END IF;
	END;
	$$ LANGUAGE plpgsql SECURITY DEFINER`,
}

// InitSchema initializes the database schema
func InitSchema(ctx context.Context, db Database) error {
	return db.WithTx(ctx, func(tx pgx.Tx) error {
		// Create schema version table if it doesn't exist
		_, err := tx.Exec(ctx, `
			CREATE TABLE IF NOT EXISTS schema_versions (
				version INTEGER PRIMARY KEY,
				applied_at TIMESTAMP WITH TIME ZONE NOT NULL DEFAULT NOW()
			)
		`)
		if err != nil {
			return fmt.Errorf("failed to create schema versions table: %w", err)
		}

		// Check current version
		var currentVersion int
		err = tx.QueryRow(ctx, "SELECT COALESCE(MAX(version), 0) FROM schema_versions").Scan(&currentVersion)
		if err != nil {
			return fmt.Errorf("failed to get current schema version: %w", err)
		}

		// Apply missing migrations
		for version, migration := range Schema {
			version++ // 1-based versioning
			if version > currentVersion {
				if _, err := tx.Exec(ctx, migration); err != nil {
					return fmt.Errorf("failed to apply migration %d: %w", version, err)
				}
				if _, err := tx.Exec(ctx, "INSERT INTO schema_versions (version) VALUES ($1)", version); err != nil {
					return fmt.Errorf("failed to record migration %d: %w", version, err)
				}
			}
		}

		return nil
	})
}
