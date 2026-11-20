CREATE TABLE users (
  id INT PRIMARY KEY AUTO_INCREMENT,
  email VARCHAR(255) NOT NULL,
  nickname VARCHAR(50) DEFAULT 'guest',
  created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
  INDEX idx_email (email)
);

INSERT INTO users (email, nickname) VALUES
  ('a@x.com', 'A'),
  ('b@x.com', 'B'),
  ('c@x.com', 'C');
