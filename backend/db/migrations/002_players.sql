CREATE TABLE IF NOT EXISTS players (
    id              UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    tournament_id   UUID REFERENCES tournaments(id) ON DELETE CASCADE,
    steam64_id      TEXT NOT NULL,
    vanity_url      TEXT,
    steam_name      TEXT,
    avatar_url      TEXT,
    faceit_elo      INT,
    faceit_level    INT,
    premier_rating  INT,
    skill_score     FLOAT,
    is_banned       BOOLEAN DEFAULT false,
    ban_info        JSONB,
    privacy_mode    TEXT,
    raw_stats       JSONB,
    fetched_at      TIMESTAMPTZ,
    created_at      TIMESTAMPTZ DEFAULT now(),
    UNIQUE (tournament_id, steam64_id)
);
