-- Reverse of 000006: remove the `connect` client and its redirect URIs.
DELETE FROM oauth_client_redirect_uris WHERE client_id = 'connect';
DELETE FROM oauth_clients WHERE id = 'connect';
