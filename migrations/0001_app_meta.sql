-- +goose Up
CREATE TABLE app_meta (
  k VARCHAR(64)  NOT NULL PRIMARY KEY,
  v VARCHAR(255) NOT NULL
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_0900_ai_ci;
INSERT INTO app_meta (k, v) VALUES ('schema_epoch', '1');

-- +goose Down
DROP TABLE app_meta;
