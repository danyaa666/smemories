-- +goose Up
-- T-061: anonymous "did the PDF look right?" counters of the print screen, one row per (browser family, verdict).
-- No personal data: no user, book, IP or user agent is stored, only how many times each answer came in.
-- Values are validated in Go (no ENUM): browser_family is the list in internal/printfeedback, verdict is yes or no.
CREATE TABLE print_feedback_tab (
  id             BIGINT UNSIGNED NOT NULL AUTO_INCREMENT,
  browser_family VARCHAR(24)     NOT NULL,
  verdict        VARCHAR(8)      NOT NULL,
  hits           BIGINT UNSIGNED NOT NULL DEFAULT 0 COMMENT 'answers received',
  created_at     BIGINT          NOT NULL COMMENT 'Unix ms',
  updated_at     BIGINT          NOT NULL COMMENT 'Unix ms',
  PRIMARY KEY (id),
  UNIQUE KEY uq_print_feedback_tab_family_verdict (browser_family, verdict)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;

-- +goose Down
DROP TABLE print_feedback_tab;
