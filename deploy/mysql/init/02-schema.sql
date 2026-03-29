USE user;

CREATE TABLE IF NOT EXISTS users (
  id BIGINT NOT NULL AUTO_INCREMENT,
  username VARCHAR(255) NOT NULL,
  password_hash VARCHAR(255) NOT NULL,
  created_at DATETIME NOT NULL,
  updated_at DATETIME NOT NULL,
  PRIMARY KEY (id)
);

CREATE TABLE IF NOT EXISTS user_addresses (
  id BIGINT NOT NULL AUTO_INCREMENT,
  user_id BIGINT NOT NULL,
  consignee VARCHAR(64) NOT NULL,
  phone VARCHAR(32) NOT NULL DEFAULT '',
  province VARCHAR(64) NOT NULL DEFAULT '',
  city VARCHAR(64) NOT NULL DEFAULT '',
  detail VARCHAR(255) NOT NULL DEFAULT '',
  is_default TINYINT(1) NOT NULL DEFAULT 0,
  created_at DATETIME NOT NULL,
  updated_at DATETIME NOT NULL,
  PRIMARY KEY (id),
  KEY idx_user_id (user_id)
);

USE goods;

CREATE TABLE IF NOT EXISTS goods (
  id BIGINT NOT NULL AUTO_INCREMENT,
  title VARCHAR(255) NOT NULL,
  intro VARCHAR(255) NOT NULL,
  created_at DATETIME NOT NULL,
  updated_at DATETIME NOT NULL,
  PRIMARY KEY (id)
);

CREATE TABLE IF NOT EXISTS orders (
  id BIGINT NOT NULL AUTO_INCREMENT,
  sn VARCHAR(255) NOT NULL,
  gid BIGINT NOT NULL,
  uid BIGINT NOT NULL,
  created_at DATETIME NOT NULL,
  updated_at DATETIME NOT NULL,
  PRIMARY KEY (id)
);

CREATE TABLE IF NOT EXISTS order_goods (
  id BIGINT NOT NULL AUTO_INCREMENT,
  order_id BIGINT NOT NULL,
  goods_id BIGINT NOT NULL,
  goods_title VARCHAR(255) NOT NULL,
  created_at DATETIME NOT NULL,
  updated_at DATETIME NOT NULL,
  PRIMARY KEY (id)
);

CREATE TABLE IF NOT EXISTS adama_stock_reservations (
  order_id BIGINT NOT NULL,
  goods_id BIGINT NOT NULL,
  amount BIGINT NOT NULL DEFAULT 1,
  status VARCHAR(32) NOT NULL,
  created_at DATETIME NOT NULL,
  updated_at DATETIME NOT NULL,
  PRIMARY KEY (order_id),
  KEY idx_goods_status (goods_id, status)
);

CREATE TABLE IF NOT EXISTS adama_goods (
  id BIGINT NOT NULL AUTO_INCREMENT,
  goods_id BIGINT NOT NULL,
  adama_price DECIMAL(10,2) NOT NULL,
  stock_count BIGINT NOT NULL,
  start_date DATETIME NOT NULL,
  end_date DATETIME NOT NULL,
  PRIMARY KEY (id),
  UNIQUE KEY uk_goods_id (goods_id)
);

USE `order`;

CREATE TABLE IF NOT EXISTS orders (
  id BIGINT NOT NULL AUTO_INCREMENT,
  sn VARCHAR(255) NOT NULL,
  gid BIGINT NOT NULL,
  uid BIGINT NOT NULL,
  created_at DATETIME NOT NULL,
  updated_at DATETIME NOT NULL,
  PRIMARY KEY (id)
);

CREATE TABLE IF NOT EXISTS adama_goods (
  id BIGINT NOT NULL AUTO_INCREMENT,
  goods_id BIGINT NOT NULL,
  adama_price DECIMAL(10,2) NOT NULL,
  stock_count BIGINT NOT NULL,
  start_date DATETIME NOT NULL,
  end_date DATETIME NOT NULL,
  PRIMARY KEY (id),
  UNIQUE KEY uk_goods_id (goods_id)
);

CREATE TABLE IF NOT EXISTS adama_orders (
  id BIGINT NOT NULL AUTO_INCREMENT,
  user_id BIGINT NOT NULL,
  order_id BIGINT NOT NULL,
  goods_id BIGINT NOT NULL,
  PRIMARY KEY (id),
  UNIQUE KEY uk_order_id (order_id),
  KEY idx_user_goods (user_id, goods_id)
);

CREATE TABLE IF NOT EXISTS adama_order_workflows (
  order_id BIGINT NOT NULL,
  user_id BIGINT NOT NULL,
  goods_id BIGINT NOT NULL,
  amount BIGINT NOT NULL DEFAULT 1,
  stock_token VARCHAR(128) NOT NULL DEFAULT '',
  status VARCHAR(32) NOT NULL,
  stock_status VARCHAR(32) NOT NULL,
  cache_status VARCHAR(32) NOT NULL,
  sync_status VARCHAR(32) NOT NULL,
  kafka_attempts INT NOT NULL DEFAULT 0,
  last_error VARCHAR(255) NOT NULL DEFAULT '',
  expire_at DATETIME NOT NULL,
  paid_at DATETIME NULL,
  created_at DATETIME NOT NULL,
  updated_at DATETIME NOT NULL,
  PRIMARY KEY (order_id),
  KEY idx_status_expire (status, expire_at),
  KEY idx_sync_status (sync_status, updated_at),
  KEY idx_sync_status_status_updated (sync_status, status, updated_at),
  KEY idx_status_stock_updated (status, stock_status, updated_at),
  KEY idx_status_cache_updated (status, cache_status, updated_at)
);

USE dtm_barrier;

CREATE TABLE IF NOT EXISTS trans_global (
  id BIGINT(22) NOT NULL AUTO_INCREMENT,
  gid VARCHAR(128) NOT NULL,
  trans_type VARCHAR(45) NOT NULL,
  status VARCHAR(12) NOT NULL,
  query_prepared VARCHAR(1024) NOT NULL,
  protocol VARCHAR(45) NOT NULL,
  create_time DATETIME DEFAULT NULL,
  update_time DATETIME DEFAULT NULL,
  finish_time DATETIME DEFAULT NULL,
  rollback_time DATETIME DEFAULT NULL,
  options VARCHAR(1024) DEFAULT '',
  custom_data VARCHAR(1024) DEFAULT '',
  next_cron_interval INT(11) DEFAULT NULL,
  next_cron_time DATETIME DEFAULT NULL,
  owner VARCHAR(128) NOT NULL DEFAULT '',
  ext_data TEXT,
  result VARCHAR(1024) DEFAULT '',
  rollback_reason VARCHAR(1024) DEFAULT '',
  PRIMARY KEY (id),
  UNIQUE KEY uk_gid (gid),
  KEY idx_owner (owner),
  KEY idx_status_next_cron_time (status, next_cron_time)
);

CREATE TABLE IF NOT EXISTS trans_branch_op (
  id BIGINT(22) NOT NULL AUTO_INCREMENT,
  gid VARCHAR(128) NOT NULL,
  url VARCHAR(1024) NOT NULL,
  data TEXT,
  bin_data BLOB,
  branch_id VARCHAR(128) NOT NULL,
  op VARCHAR(45) NOT NULL,
  status VARCHAR(45) NOT NULL,
  finish_time DATETIME DEFAULT NULL,
  rollback_time DATETIME DEFAULT NULL,
  create_time DATETIME DEFAULT NULL,
  update_time DATETIME DEFAULT NULL,
  PRIMARY KEY (id),
  UNIQUE KEY uk_gid_branch_op (gid, branch_id, op)
);

CREATE TABLE IF NOT EXISTS kv (
  id BIGINT(22) NOT NULL AUTO_INCREMENT,
  cat VARCHAR(45) NOT NULL,
  k VARCHAR(128) NOT NULL,
  v TEXT,
  version BIGINT(22) DEFAULT 1,
  create_time DATETIME DEFAULT NULL,
  update_time DATETIME DEFAULT NULL,
  PRIMARY KEY (id),
  UNIQUE KEY uk_cat_k (cat, k)
);

CREATE TABLE IF NOT EXISTS barrier (
  id BIGINT(22) NOT NULL AUTO_INCREMENT,
  trans_type VARCHAR(45) NOT NULL,
  gid VARCHAR(128) NOT NULL,
  branch_id VARCHAR(128) NOT NULL,
  op VARCHAR(45) NOT NULL,
  barrier_id VARCHAR(45) NOT NULL,
  reason VARCHAR(45) NOT NULL,
  create_time DATETIME DEFAULT NULL,
  update_time DATETIME DEFAULT NULL,
  PRIMARY KEY (id),
  UNIQUE KEY uniq_barrier (trans_type, gid, branch_id, op, barrier_id)
);
