USE user;

INSERT INTO users (id, username, password_hash, created_at, updated_at)
VALUES
  (88, 'bench-user-88', 'e10adc3949ba59abbe56e057f20f883e', NOW(), NOW()),
  (89, 'bench-user-89', 'e10adc3949ba59abbe56e057f20f883e', NOW(), NOW())
ON DUPLICATE KEY UPDATE
  username = VALUES(username),
  password_hash = VALUES(password_hash),
  updated_at = VALUES(updated_at);

INSERT INTO user_addresses (id, user_id, consignee, phone, province, city, detail, is_default, created_at, updated_at)
VALUES
  (1, 88, 'Benchmark User', '13800000000', 'Shanghai', 'Shanghai', 'Benchmark Road 1', 1, NOW(), NOW()),
  (2, 88, 'Benchmark Office', '13800000001', 'Shanghai', 'Shanghai', 'Benchmark Road 2', 0, NOW(), NOW()),
  (3, 89, 'Load Test User', '13800000002', 'Beijing', 'Beijing', 'Pressure Ave 9', 1, NOW(), NOW())
ON DUPLICATE KEY UPDATE
  consignee = VALUES(consignee),
  phone = VALUES(phone),
  province = VALUES(province),
  city = VALUES(city),
  detail = VALUES(detail),
  is_default = VALUES(is_default),
  updated_at = VALUES(updated_at);

USE goods;

INSERT INTO goods (id, title, intro, created_at, updated_at)
VALUES
  (1, 'adama-phone', 'benchmark phone for seckill flow', NOW(), NOW()),
  (2, 'adama-laptop', 'benchmark laptop baseline item', NOW(), NOW()),
  (3, 'adama-headset', 'benchmark headset baseline item', NOW(), NOW()),
  (4, 'adama-keyboard', 'benchmark keyboard baseline item', NOW(), NOW()),
  (5, 'adama-monitor', 'benchmark monitor baseline item', NOW(), NOW())
ON DUPLICATE KEY UPDATE
  title = VALUES(title),
  intro = VALUES(intro),
  updated_at = VALUES(updated_at);

INSERT INTO orders (id, sn, gid, uid, created_at, updated_at)
VALUES
  (1001, 'ORD-1001', 1, 88, NOW(), NOW()),
  (1002, 'ORD-1002', 2, 88, NOW(), NOW()),
  (1003, 'ORD-1003', 3, 89, NOW(), NOW())
ON DUPLICATE KEY UPDATE
  sn = VALUES(sn),
  gid = VALUES(gid),
  uid = VALUES(uid),
  updated_at = VALUES(updated_at);

INSERT INTO order_goods (id, order_id, goods_id, goods_title, created_at, updated_at)
VALUES
  (2001, 1001, 1, 'adama-phone', NOW(), NOW()),
  (2002, 1002, 2, 'adama-laptop', NOW(), NOW()),
  (2003, 1003, 3, 'adama-headset', NOW(), NOW())
ON DUPLICATE KEY UPDATE
  order_id = VALUES(order_id),
  goods_id = VALUES(goods_id),
  goods_title = VALUES(goods_title),
  updated_at = VALUES(updated_at);

INSERT INTO adama_goods (id, goods_id, adama_price, stock_count, start_date, end_date)
VALUES
  (1, 1, 99.00, 100, DATE_SUB(NOW(), INTERVAL 1 HOUR), DATE_ADD(NOW(), INTERVAL 7 DAY)),
  (2, 2, 199.00, 50, DATE_SUB(NOW(), INTERVAL 1 HOUR), DATE_ADD(NOW(), INTERVAL 7 DAY))
ON DUPLICATE KEY UPDATE
  goods_id = VALUES(goods_id),
  adama_price = VALUES(adama_price),
  stock_count = VALUES(stock_count),
  start_date = VALUES(start_date),
  end_date = VALUES(end_date);

USE `order`;

INSERT INTO orders (id, sn, gid, uid, created_at, updated_at)
VALUES
  (1001, 'ORD-1001', 1, 88, NOW(), NOW()),
  (1002, 'ORD-1002', 2, 88, NOW(), NOW()),
  (1003, 'ORD-1003', 3, 89, NOW(), NOW())
ON DUPLICATE KEY UPDATE
  sn = VALUES(sn),
  gid = VALUES(gid),
  uid = VALUES(uid),
  updated_at = VALUES(updated_at);

INSERT INTO adama_goods (id, goods_id, adama_price, stock_count, start_date, end_date)
VALUES
  (1, 1, 99.00, 100, DATE_SUB(NOW(), INTERVAL 1 HOUR), DATE_ADD(NOW(), INTERVAL 7 DAY)),
  (2, 2, 199.00, 50, DATE_SUB(NOW(), INTERVAL 1 HOUR), DATE_ADD(NOW(), INTERVAL 7 DAY))
ON DUPLICATE KEY UPDATE
  goods_id = VALUES(goods_id),
  adama_price = VALUES(adama_price),
  stock_count = VALUES(stock_count),
  start_date = VALUES(start_date),
  end_date = VALUES(end_date);

INSERT INTO adama_orders (id, user_id, order_id, goods_id)
VALUES
  (3001, 88, 900001, 1)
ON DUPLICATE KEY UPDATE
  user_id = VALUES(user_id),
  order_id = VALUES(order_id),
  goods_id = VALUES(goods_id);

INSERT INTO adama_order_workflows (
  order_id, user_id, goods_id, amount, stock_token, status, stock_status, cache_status, sync_status,
  kafka_attempts, last_error, expire_at, paid_at, created_at, updated_at
)
VALUES
  (900001, 88, 1, 1, 'seed-stock-token', 'pending_pay', 'reserved', 'reserved', 'synced',
   1, '', DATE_ADD(NOW(), INTERVAL 30 MINUTE), NULL, NOW(), NOW())
ON DUPLICATE KEY UPDATE
  user_id = VALUES(user_id),
  goods_id = VALUES(goods_id),
  amount = VALUES(amount),
  stock_token = VALUES(stock_token),
  status = VALUES(status),
  stock_status = VALUES(stock_status),
  cache_status = VALUES(cache_status),
  sync_status = VALUES(sync_status),
  kafka_attempts = VALUES(kafka_attempts),
  last_error = VALUES(last_error),
  expire_at = VALUES(expire_at),
  paid_at = VALUES(paid_at),
  updated_at = VALUES(updated_at);
