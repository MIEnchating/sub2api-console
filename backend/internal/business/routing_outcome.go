package business

// Select each stream's newest relevant outcome through its existing index,
// then compare the two. Restore outcomes must supersede old write failures,
// including for existing databases whose partial index excludes restores.
const latestRoutingOutcomeSQL = `SELECT source_id FROM (
	SELECT * FROM (
		SELECT recent.source_id,recent.created_at FROM operation_audit recent INDEXED BY ix_operation_audit_apply_error_recent
		WHERE recent.operation_type IN ('routing.writeback','cleanup.delete') AND recent.object_id=a.id
		AND (recent.state='failed' OR recent.readback_confirmed=1)` + excludeLegacyPolicyChangeOutcomeSQL + `
		ORDER BY recent.created_at DESC,
		CASE WHEN recent.source_id < 0 THEN 0 ELSE 1 END,
		CASE WHEN recent.source_id < 0 THEN recent.source_id END ASC,
		CASE WHEN recent.source_id >= 0 THEN recent.source_id END DESC LIMIT 1
	) UNION ALL SELECT * FROM (
		SELECT recent.source_id,recent.created_at FROM operation_audit recent INDEXED BY ix_operation_audit_type_object_recent
		WHERE recent.operation_type='routing.restore' AND recent.object_id=a.id
		AND (recent.state='failed' OR (recent.state='succeeded' AND recent.readback_confirmed=1))
		ORDER BY recent.created_at DESC,
		CASE WHEN recent.source_id < 0 THEN 0 ELSE 1 END,
		CASE WHEN recent.source_id < 0 THEN recent.source_id END ASC,
		CASE WHEN recent.source_id >= 0 THEN recent.source_id END DESC LIMIT 1
	)
) ORDER BY created_at DESC,
CASE WHEN source_id < 0 THEN 0 ELSE 1 END,
CASE WHEN source_id < 0 THEN source_id END ASC,
CASE WHEN source_id >= 0 THEN source_id END DESC LIMIT 1`
