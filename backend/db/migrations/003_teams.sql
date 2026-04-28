CREATE TABLE IF NOT EXISTS teams (
    id              UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    tournament_id   UUID REFERENCES tournaments(id) ON DELETE CASCADE,
    name            TEXT NOT NULL,
    avg_skill_score FLOAT DEFAULT 0,
    created_at      TIMESTAMPTZ DEFAULT now()
);

CREATE TABLE IF NOT EXISTS team_players (
    team_id         UUID REFERENCES teams(id) ON DELETE CASCADE,
    player_id       UUID REFERENCES players(id) ON DELETE CASCADE,
    manually_moved  BOOLEAN DEFAULT false,
    PRIMARY KEY (team_id, player_id)
);
