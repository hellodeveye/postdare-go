USE postdare_go;

SET SESSION group_concat_max_len = 1048576;

UPDATE projects p
JOIN (
  SELECT
    p.id,
    CONCAT(
      '[',
      GROUP_CONCAT(
        CASE
          WHEN JSON_UNQUOTE(JSON_EXTRACT(stage.stage_json, '$.type')) = 'outbound_webhook'
            AND COALESCE(JSON_UNQUOTE(JSON_EXTRACT(stage.stage_json, '$.config.template')), '') IN ('', 'dingtalk_text', 'wecom_text', 'feishu_text')
            AND (
              LOWER(COALESCE(JSON_UNQUOTE(JSON_EXTRACT(stage.stage_json, '$.config.url')), '')) LIKE '%feishu%'
              OR LOWER(COALESCE(JSON_UNQUOTE(JSON_EXTRACT(stage.stage_json, '$.config.url')), '')) LIKE '%larksuite%'
              OR LOWER(COALESCE(JSON_UNQUOTE(JSON_EXTRACT(stage.stage_json, '$.config.url')), '')) LIKE '%qyapi.weixin%'
              OR LOWER(COALESCE(JSON_UNQUOTE(JSON_EXTRACT(stage.stage_json, '$.config.url')), '')) LIKE '%weixin%'
              OR LOWER(COALESCE(JSON_UNQUOTE(JSON_EXTRACT(stage.stage_json, '$.config.url')), '')) LIKE '%wechat%'
              OR LOWER(COALESCE(JSON_UNQUOTE(JSON_EXTRACT(stage.stage_json, '$.config.url')), '')) LIKE '%wecom%'
            )
          THEN JSON_SET(
            JSON_SET(stage.stage_json, '$.config', COALESCE(JSON_EXTRACT(stage.stage_json, '$.config'), JSON_OBJECT())),
            '$.config.template',
            CASE
              WHEN LOWER(COALESCE(JSON_UNQUOTE(JSON_EXTRACT(stage.stage_json, '$.config.url')), '')) LIKE '%feishu%'
                OR LOWER(COALESCE(JSON_UNQUOTE(JSON_EXTRACT(stage.stage_json, '$.config.url')), '')) LIKE '%larksuite%'
              THEN 'feishu_text'
              ELSE 'wecom_text'
            END
          )
          ELSE stage.stage_json
        END
        ORDER BY stage.stage_ord
        SEPARATOR ','
      ),
      ']'
    ) AS deploy_stages
  FROM projects p
  JOIN JSON_TABLE(
    p.deploy_stages,
    '$[*]' COLUMNS (
      stage_ord FOR ORDINALITY,
      stage_json JSON PATH '$'
    )
  ) AS stage
  WHERE JSON_LENGTH(p.deploy_stages) > 0
    AND JSON_SEARCH(p.deploy_stages, 'one', 'outbound_webhook', NULL, '$[*].type') IS NOT NULL
  GROUP BY p.id
) rebuilt ON rebuilt.id = p.id
SET p.deploy_stages = rebuilt.deploy_stages;
