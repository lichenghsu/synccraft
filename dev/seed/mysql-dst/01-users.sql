CREATE TABLE users (
  id INT PRIMARY KEY AUTO_INCREMENT,
  email VARCHAR(100) NOT NULL
);

INSERT INTO users (id, email) VALUES
  (1, 'a@x.com'),
  (2, 'OLD@x.com'),
  (9, 'z@x.com');
