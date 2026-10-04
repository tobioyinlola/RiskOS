-- +goose Up
-- Every workspace has a compliance profile (ADR-0006): created with the workspace, so the rules
-- page always has a pack to show. An automatic profile uses the Nigeria pack with the licence
-- type "other" until a person confirms the institution type, and starts with the regulatory
-- controls the law requires at their defaults and every risk signal off until a compliance
-- person activates it. Profiles people set up before keep the pack's defaults for signals.
ALTER TABLE tenant_compliance ADD COLUMN signal_default text NOT NULL DEFAULT 'pack'
  CHECK (signal_default IN ('pack', 'off'));

-- +goose StatementBegin
CREATE FUNCTION tenant_compliance_default() RETURNS trigger
  LANGUAGE plpgsql SECURITY DEFINER
  SET search_path = pg_catalog, public
  AS $$
BEGIN
  INSERT INTO tenant_compliance (tenant_id, pack_id, pack_version, licence_type, capabilities,
                                 updated_at, updated_by, signal_default)
  SELECT NEW.id, p.id, p.version, 'other', '{}', now(), 'system:default', 'off'
  FROM rule_packs p
  WHERE p.jurisdiction = 'NG'
  ORDER BY string_to_array(p.version, '.')::int[] DESC
  LIMIT 1
  ON CONFLICT (tenant_id) DO NOTHING;
  RETURN NEW;
END
$$;
-- +goose StatementEnd
REVOKE ALL ON FUNCTION tenant_compliance_default() FROM PUBLIC;
CREATE TRIGGER tenants_compliance_default AFTER INSERT ON tenants
  FOR EACH ROW EXECUTE FUNCTION tenant_compliance_default();

-- Workspaces that existed before: the same automatic profile. (A database with no pack loaded
-- yet gets them when the pack loader runs: rules.LoadPacks.)
INSERT INTO tenant_compliance (tenant_id, pack_id, pack_version, licence_type, capabilities,
                               updated_at, updated_by, signal_default)
SELECT t.id, p.id, p.version, 'other', '{}', now(), 'system:default', 'off'
FROM tenants t
CROSS JOIN LATERAL (
  SELECT id, version FROM rule_packs WHERE jurisdiction = 'NG'
  ORDER BY string_to_array(version, '.')::int[] DESC LIMIT 1
) p
WHERE NOT EXISTS (SELECT 1 FROM tenant_compliance c WHERE c.tenant_id = t.id);

-- +goose Down
DROP TRIGGER tenants_compliance_default ON tenants;
DROP FUNCTION tenant_compliance_default();
ALTER TABLE tenant_compliance DROP COLUMN signal_default;
