-- Insert test CDK keys
INSERT OR IGNORE INTO cd_keys (code, key_type, amount, status, created_by_user_id, expires_at, description)
VALUES 
  ('GPTR-TEST-5X-001', 'gpt-5x', 100, 'active', 1, datetime('now', '+30 days'), 'Test 5x CDK'),
  ('GPTR-TEST-5X-002', 'gpt-5x', 100, 'active', 1, datetime('now', '+30 days'), 'Test 5x CDK'),
  ('GPTR-TEST-20X-001', 'gpt-20x', 500, 'active', 1, datetime('now', '+30 days'), 'Test 20x CDK'),
  ('GPTR-TEST-20X-002', 'gpt-20x', 500, 'active', 1, datetime('now', '+30 days'), 'Test 20x CDK'),
  ('GPTR-USED-001', 'gpt-5x', 100, 'used', 1, datetime('now', '+30 days'), 'Already used'),
  ('GPTR-EXPIRED-001', 'gpt-5x', 100, 'expired', 1, datetime('now', '-1 days'), 'Expired');
