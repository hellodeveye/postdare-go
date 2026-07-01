USE postdare_go;

SET @schema_name = DATABASE();

SET @has_deploy_stages = (
  SELECT COUNT(*)
  FROM information_schema.columns
  WHERE table_schema = @schema_name
    AND table_name = 'projects'
    AND column_name = 'deploy_stages'
);
SET @sql = IF(
  @has_deploy_stages = 0,
  'ALTER TABLE projects ADD COLUMN deploy_stages JSON AFTER rollback_cmd',
  'SELECT 1'
);
PREPARE stmt FROM @sql;
EXECUTE stmt;
DEALLOCATE PREPARE stmt;

SET @has_stages = (
  SELECT COUNT(*)
  FROM information_schema.columns
  WHERE table_schema = @schema_name
    AND table_name = 'projects'
    AND column_name = 'stages'
);
SET @sql = IF(
  @has_stages > 0,
  'UPDATE projects SET deploy_stages = stages WHERE deploy_stages IS NULL AND stages IS NOT NULL',
  'SELECT 1'
);
PREPARE stmt FROM @sql;
EXECUTE stmt;
DEALLOCATE PREPARE stmt;

SET @sql = IF(@has_stages > 0, 'ALTER TABLE projects DROP COLUMN stages', 'SELECT 1');
PREPARE stmt FROM @sql;
EXECUTE stmt;
DEALLOCATE PREPARE stmt;

SET @has_column = (
  SELECT COUNT(*)
  FROM information_schema.columns
  WHERE table_schema = @schema_name
    AND table_name = 'projects'
    AND column_name = 'pull_cmd'
);
SET @sql = IF(@has_column > 0, 'ALTER TABLE projects DROP COLUMN pull_cmd', 'SELECT 1');
PREPARE stmt FROM @sql;
EXECUTE stmt;
DEALLOCATE PREPARE stmt;

SET @has_column = (
  SELECT COUNT(*)
  FROM information_schema.columns
  WHERE table_schema = @schema_name
    AND table_name = 'projects'
    AND column_name = 'unit_test_cmd'
);
SET @sql = IF(@has_column > 0, 'ALTER TABLE projects DROP COLUMN unit_test_cmd', 'SELECT 1');
PREPARE stmt FROM @sql;
EXECUTE stmt;
DEALLOCATE PREPARE stmt;

SET @has_column = (
  SELECT COUNT(*)
  FROM information_schema.columns
  WHERE table_schema = @schema_name
    AND table_name = 'projects'
    AND column_name = 'integration_test_cmd'
);
SET @sql = IF(@has_column > 0, 'ALTER TABLE projects DROP COLUMN integration_test_cmd', 'SELECT 1');
PREPARE stmt FROM @sql;
EXECUTE stmt;
DEALLOCATE PREPARE stmt;

SET @has_column = (
  SELECT COUNT(*)
  FROM information_schema.columns
  WHERE table_schema = @schema_name
    AND table_name = 'projects'
    AND column_name = 'build_cmd'
);
SET @sql = IF(@has_column > 0, 'ALTER TABLE projects DROP COLUMN build_cmd', 'SELECT 1');
PREPARE stmt FROM @sql;
EXECUTE stmt;
DEALLOCATE PREPARE stmt;

SET @has_column = (
  SELECT COUNT(*)
  FROM information_schema.columns
  WHERE table_schema = @schema_name
    AND table_name = 'projects'
    AND column_name = 'deploy_cmd'
);
SET @sql = IF(@has_column > 0, 'ALTER TABLE projects DROP COLUMN deploy_cmd', 'SELECT 1');
PREPARE stmt FROM @sql;
EXECUTE stmt;
DEALLOCATE PREPARE stmt;
