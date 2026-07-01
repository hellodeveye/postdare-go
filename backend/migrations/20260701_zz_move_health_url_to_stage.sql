USE postdare_go;

SET @schema_name = DATABASE();
SET SESSION group_concat_max_len = 1048576;

UPDATE projects
SET deploy_stages = JSON_ARRAY()
WHERE deploy_stages IS NULL;

SET @has_health_url = (
  SELECT COUNT(*)
  FROM information_schema.columns
  WHERE table_schema = @schema_name
    AND table_name = 'projects'
    AND column_name = 'health_url'
);

SET @sql = IF(
  @has_health_url > 0,
  'UPDATE projects p
   JOIN (
     SELECT
       p.id,
       CONCAT(
         ''['',
         GROUP_CONCAT(
           CASE
             WHEN JSON_UNQUOTE(JSON_EXTRACT(stage.stage_json, ''$.type'')) = ''health_check''
               AND COALESCE(JSON_UNQUOTE(JSON_EXTRACT(stage.stage_json, ''$.config.url'')), '''') = ''''
             THEN JSON_SET(
               JSON_SET(stage.stage_json, ''$.config'', COALESCE(JSON_EXTRACT(stage.stage_json, ''$.config''), JSON_OBJECT())),
               ''$.config.url'',
               p.health_url
             )
             ELSE stage.stage_json
           END
           ORDER BY stage.stage_ord
           SEPARATOR '',''
         ),
         '']''
       ) AS deploy_stages
     FROM projects p
     JOIN JSON_TABLE(
       p.deploy_stages,
       ''$[*]'' COLUMNS (
         stage_ord FOR ORDINALITY,
         stage_json JSON PATH ''$''
       )
     ) AS stage
     WHERE p.health_url IS NOT NULL
       AND p.health_url <> ''''
       AND JSON_LENGTH(p.deploy_stages) > 0
       AND JSON_SEARCH(p.deploy_stages, ''one'', ''health_check'', NULL, ''$[*].type'') IS NOT NULL
     GROUP BY p.id
   ) rebuilt ON rebuilt.id = p.id
   SET p.deploy_stages = rebuilt.deploy_stages',
  'SELECT 1'
);
PREPARE stmt FROM @sql;
EXECUTE stmt;
DEALLOCATE PREPARE stmt;

SET @sql = IF(
  @has_health_url > 0,
  'UPDATE projects
   SET deploy_stages = JSON_ARRAY_APPEND(
     deploy_stages,
     ''$'',
     JSON_EXTRACT(
       CONCAT(
         ''{"name":"health_check","type":"health_check","enabled":true,"config":{"url":'',
         JSON_QUOTE(health_url),
         ''}}''
       ),
       ''$''
     )
   )
   WHERE health_url IS NOT NULL
     AND health_url <> ''''
     AND JSON_SEARCH(deploy_stages, ''one'', ''health_check'', NULL, ''$[*].type'') IS NULL',
  'SELECT 1'
);
PREPARE stmt FROM @sql;
EXECUTE stmt;
DEALLOCATE PREPARE stmt;

SET @sql = IF(@has_health_url > 0, 'ALTER TABLE projects DROP COLUMN health_url', 'SELECT 1');
PREPARE stmt FROM @sql;
EXECUTE stmt;
DEALLOCATE PREPARE stmt;
