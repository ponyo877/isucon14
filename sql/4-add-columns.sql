ALTER TABLE chairs
ADD COLUMN is_completed TINYINT(1) NOT NULL DEFAULT 1 COMMENT 'ライドが完了したかどうか',
ADD INDEX idx_is_completed (is_completed);