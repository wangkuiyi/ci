CREATE TYPE BuildStatus AS ENUM (
    'queued',
    'running',
    'success',
    'error',
    'failed'
);

CREATE TYPE OutputChannel AS ENUM (
    'stdout',
    'stderr'
);

CREATE TABLE PushEvents (
    head VARCHAR(64) PRIMARY KEY NOT NULL,
    ref TEXT NOT NULL,
    clone_url TEXT NOT NULL,
    createTime TIMESTAMP WITHOUT TIME ZONE DEFAULT (now() AT TIME ZONE 'utc')
);

CREATE TABLE Builds (
    id BIGSERIAL PRIMARY KEY NOT NULL,
    status BuildStatus NOT NULL DEFAULT 'queued',
    outputs TEXT[],
    outputChannels OutputChannel[]
);


-- Relations
CREATE TABLE PushBuilds (
    push_head VARCHAR(64) NOT NULL REFERENCES PushEvents(head),
    build_id BIGINT NOT NULL REFERENCES Builds(id)
);


-- Functions

CREATE FUNCTION new_push_event(head VARCHAR(64), ref TEXT, clone_url TEXT)
RETURNS BIGINT AS $$
DECLARE bid BIGINT;
BEGIN
    INSERT INTO PushEvents(head, ref, clone_url) VALUES($1, $2, $3);
    INSERT INTO Builds DEFAULT VALUES;
    SELECT LASTVAL() INTO bid;
    INSERT INTO PushBuilds(push_head, build_id) VALUES($1, bid);
    RETURN bid;
END;
$$ LANGUAGE plpgsql