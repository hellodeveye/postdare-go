USE postdare_go;

SET @schema_name = DATABASE();
SET SESSION group_concat_max_len = 1048576;

UPDATE projects
SET deploy_stages = JSON_ARRAY()
WHERE deploy_stages IS NULL;

SET @has_default_outbound_webhook_url = (
  SELECT COUNT(*)
  FROM information_schema.columns
  WHERE table_schema = @schema_name
    AND table_name = 'projects'
    AND column_name = 'default_outbound_webhook_url'
);

SET @sql = IF(
  @has_default_outbound_webhook_url > 0,
  'UPDATE projects p
   JOIN (
     SELECT
       p.id,
       CONCAT(
         ''['',
         GROUP_CONCAT(
           CASE
             WHEN JSON_UNQUOTE(JSON_EXTRACT(stage.stage_json, ''$.type'')) = ''outbound_webhook''
               AND COALESCE(JSON_UNQUOTE(JSON_EXTRACT(stage.stage_json, ''$.config.url'')), '''') = ''''
             THEN JSON_SET(
               JSON_SET(
                 JSON_SET(stage.stage_json, ''$.config'', COALESCE(JSON_EXTRACT(stage.stage_json, ''$.config''), JSON_OBJECT())),
                 ''$.config.url'',
                 p.default_outbound_webhook_url
               ),
               ''$.config.template'',
               CASE
                 WHEN LOWER(p.default_outbound_webhook_url) LIKE ''%feishu%'' OR LOWER(p.default_outbound_webhook_url) LIKE ''%larksuite%'' THEN ''feishu_text''
                 WHEN LOWER(p.default_outbound_webhook_url) LIKE ''%qyapi.weixin%'' OR LOWER(p.default_outbound_webhook_url) LIKE ''%weixin%'' OR LOWER(p.default_outbound_webhook_url) LIKE ''%wechat%'' OR LOWER(p.default_outbound_webhook_url) LIKE ''%wecom%'' THEN ''wecom_text''
                 ELSE ''dingtalk_text''
               END
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
     WHERE p.default_outbound_webhook_url IS NOT NULL
       AND p.default_outbound_webhook_url <> ''''
       AND JSON_SEARCH(p.deploy_stages, ''one'', ''outbound_webhook'', NULL, ''$[*].type'') IS NOT NULL
     GROUP BY p.id
   ) rebuilt ON rebuilt.id = p.id
   SET p.deploy_stages = rebuilt.deploy_stages',
  'SELECT 1'
);
PREPARE stmt FROM @sql;
EXECUTE stmt;
DEALLOCATE PREPARE stmt;

SET @sql = IF(
  @has_default_outbound_webhook_url > 0,
  'UPDATE projects
   SET deploy_stages = JSON_ARRAY_APPEND(
     deploy_stages,
     ''$'',
     JSON_EXTRACT(
       CONCAT(
         ''{"name":"outbound_webhook","type":"outbound_webhook","enabled":true,"run_when":"always","continue_on_error":true,"config":{"url":'',
         JSON_QUOTE(default_outbound_webhook_url),
         '',"template":'',
         JSON_QUOTE(
           CASE
             WHEN LOWER(default_outbound_webhook_url) LIKE ''%feishu%'' OR LOWER(default_outbound_webhook_url) LIKE ''%larksuite%'' THEN ''feishu_text''
             WHEN LOWER(default_outbound_webhook_url) LIKE ''%qyapi.weixin%'' OR LOWER(default_outbound_webhook_url) LIKE ''%weixin%'' OR LOWER(default_outbound_webhook_url) LIKE ''%wechat%'' OR LOWER(default_outbound_webhook_url) LIKE ''%wecom%'' THEN ''wecom_text''
             ELSE ''dingtalk_text''
           END
         ),
         ''}}''
       ),
       ''$''
     )
   )
   WHERE default_outbound_webhook_url IS NOT NULL
     AND default_outbound_webhook_url <> ''''
     AND JSON_SEARCH(deploy_stages, ''one'', ''outbound_webhook'', NULL, ''$[*].type'') IS NULL',
  'SELECT 1'
);
PREPARE stmt FROM @sql;
EXECUTE stmt;
DEALLOCATE PREPARE stmt;

SET @sql = IF(
  @has_default_outbound_webhook_url > 0,
  'ALTER TABLE projects DROP COLUMN default_outbound_webhook_url',
  'SELECT 1'
);
PREPARE stmt FROM @sql;
EXECUTE stmt;
DEALLOCATE PREPARE stmt;
