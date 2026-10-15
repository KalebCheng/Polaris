-- 015 down：恢复 enable_exclusive_signin（由 max_sessions = 1 反推）
ALTER TABLE applications
    ADD COLUMN enable_exclusive_signin BOOLEAN NOT NULL DEFAULT FALSE;

UPDATE applications
   SET enable_exclusive_signin = (max_sessions = 1),
       updated_at              = NOW()
 WHERE max_sessions = 1;
