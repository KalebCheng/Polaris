UPDATE applications SET client_id = 'siming-built-in-client', updated_at = NOW()
    WHERE client_id = 'polaris-built-in-client';
UPDATE applications SET client_secret = 'siming-built-in-secret', updated_at = NOW()
    WHERE client_secret = 'polaris-built-in-secret';
