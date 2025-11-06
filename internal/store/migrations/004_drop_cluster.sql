-- Drop legacy cluster table; cluster id is now provided via environment.
SET search_path TO ploy, public;

DROP TABLE IF EXISTS cluster;

