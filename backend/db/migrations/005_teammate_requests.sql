-- A wish from one player to be drafted onto the same team as another.
-- The link is undirected for drafting: only one row may exist per pair.
CREATE TABLE IF NOT EXISTS teammate_requests (
    id              UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    tournament_id   UUID NOT NULL REFERENCES tournaments(id) ON DELETE CASCADE,
    requester_id    UUID NOT NULL REFERENCES players(id) ON DELETE CASCADE,
    requested_id    UUID NOT NULL REFERENCES players(id) ON DELETE CASCADE,
    created_at      TIMESTAMPTZ DEFAULT now(),
    CHECK (requester_id <> requested_id)
);

CREATE UNIQUE INDEX IF NOT EXISTS teammate_requests_pair_idx
    ON teammate_requests (LEAST(requester_id, requested_id), GREATEST(requester_id, requested_id));
