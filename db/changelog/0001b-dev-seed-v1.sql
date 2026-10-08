--liquibase formatted sql

--changeset config-example:0001b-dev-seed-v1 context:dev
--comment: Dev-only seed of v1 documents. Lives before the v2..v4 migrations on purpose so they get migrated. Mirrors backend/internal/settings/testdata/seed-v1.json.
INSERT INTO user_settings (user_id, settings) VALUES
('alice', '{
  "schemaVersion": 1,
  "featureFlags": ["newDashboard", "legacyReports"],
  "map": { "baseLayer": "satellite", "layers": ["traffic", "parcels"], "showLabels": true },
  "filterPresets": ["open-items", "assigned-to-me"]
}'),
('bob', '{
  "schemaVersion": 1,
  "featureFlags": [],
  "map": { "baseLayer": "streets", "layers": ["zoning"], "showLabels": false },
  "filterPresets": ["unassigned"]
}'),
('carol', '{
  "schemaVersion": 1,
  "featureFlags": ["legacyReports"],
  "map": { "baseLayer": "streets", "layers": [], "showLabels": true },
  "filterPresets": []
}');

--rollback DELETE FROM user_settings WHERE user_id IN ('alice', 'bob', 'carol');
