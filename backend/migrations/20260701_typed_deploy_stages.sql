USE postdare_go;

SET @schema_name = DATABASE();
SET SESSION group_concat_max_len = 1048576;

SET @has_notify_webhook = (
  SELECT COUNT(*)
  FROM information_schema.columns
  WHERE table_schema = @schema_name
    AND table_name = 'projects'
    AND column_name = 'notify_webhook'
);

SET @has_health_url = (
  SELECT COUNT(*)
  FROM information_schema.columns
  WHERE table_schema = @schema_name
    AND table_name = 'projects'
    AND column_name = 'health_url'
);

UPDATE projects p
JOIN (
  SELECT
    p.id,
    CONCAT(
      '[',
      GROUP_CONCAT(
        CONCAT(
          '{"name":',
          JSON_QUOTE(old_stage.stage_name),
          ',"type":"command","enabled":',
          IF(COALESCE(old_stage.stage_enabled, true), 'true', 'false'),
          ',"continue_on_error":',
          IF(COALESCE(old_stage.stage_continue_on_error, false), 'true', 'false'),
          ',"config":{"command":',
          JSON_QUOTE(COALESCE(old_stage.stage_command, '')),
          '}}'
        )
        ORDER BY old_stage.stage_ord
        SEPARATOR ','
      ),
      ']'
    ) AS typed_stages
  FROM projects p
  JOIN JSON_TABLE(
    p.deploy_stages,
    '$[*]' COLUMNS (
      stage_ord FOR ORDINALITY,
      stage_name VARCHAR(100) PATH '$.name',
      stage_command VARCHAR(8192) PATH '$.command' NULL ON EMPTY,
      stage_enabled BOOL PATH '$.enabled' NULL ON EMPTY,
      stage_continue_on_error BOOL PATH '$.continue_on_error' NULL ON EMPTY
    )
  ) AS old_stage
  WHERE JSON_LENGTH(p.deploy_stages) > 0
    AND JSON_UNQUOTE(JSON_EXTRACT(p.deploy_stages, '$[0].type')) IS NULL
  GROUP BY p.id
) converted ON converted.id = p.id
SET p.deploy_stages = converted.typed_stages;

UPDATE projects
SET deploy_stages = JSON_ARRAY()
WHERE deploy_stages IS NULL;

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

SET @sql = IF(
  @has_notify_webhook > 0,
  'UPDATE projects
   SET deploy_stages = JSON_ARRAY_APPEND(
     deploy_stages,
     ''$'',
     JSON_EXTRACT(
       CONCAT(
         ''{"name":"outbound_webhook","type":"outbound_webhook","enabled":true,"run_when":"always","continue_on_error":true,"config":{"url":'',
         JSON_QUOTE(notify_webhook),
         '',"template":'',
         JSON_QUOTE(
           CASE
             WHEN LOWER(notify_webhook) LIKE ''%feishu%'' OR LOWER(notify_webhook) LIKE ''%larksuite%'' THEN ''feishu_text''
             WHEN LOWER(notify_webhook) LIKE ''%qyapi.weixin%'' OR LOWER(notify_webhook) LIKE ''%weixin%'' OR LOWER(notify_webhook) LIKE ''%wechat%'' OR LOWER(notify_webhook) LIKE ''%wecom%'' THEN ''wecom_text''
             ELSE ''dingtalk_text''
           END
         ),
         ''}}''
       ),
       ''$''
     )
   )
   WHERE notify_webhook IS NOT NULL
     AND notify_webhook <> ''''
     AND JSON_SEARCH(deploy_stages, ''one'', ''outbound_webhook'', NULL, ''$[*].type'') IS NULL',
  'SELECT 1'
);
PREPARE stmt FROM @sql;
EXECUTE stmt;
DEALLOCATE PREPARE stmt;

SET @sql = IF(@has_notify_webhook > 0, 'ALTER TABLE projects DROP COLUMN notify_webhook', 'SELECT 1');
PREPARE stmt FROM @sql;
EXECUTE stmt;
DEALLOCATE PREPARE stmt;
