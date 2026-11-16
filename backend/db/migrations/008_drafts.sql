-- A tournament can hold several drafts, each made with its own settings.
CREATE TABLE IF NOT EXISTS drafts (
    id               UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    tournament_id    UUID NOT NULL REFERENCES tournaments(id) ON DELETE CASCADE,
    name             TEXT NOT NULL,
    -- Only the settings that were chosen; anything missing is the default.
    settings         JSONB NOT NULL DEFAULT '{}',
    -- Request id → what the draft did with it.
    request_statuses JSONB,
    created_at       TIMESTAMPTZ DEFAULT now()
);

-- The score every player had under the draft's settings when it was made.
-- A player without a row could not be scored and was left out.
CREATE TABLE IF NOT EXISTS draft_scores (
    draft_id    UUID NOT NULL REFERENCES drafts(id) ON DELETE CASCADE,
    player_id   UUID NOT NULL REFERENCES players(id) ON DELETE CASCADE,
    score       FLOAT NOT NULL,
    -- The settings could not score the player; the full model stood in.
    estimated   BOOLEAN NOT NULL DEFAULT false,
    PRIMARY KEY (draft_id, player_id)
);

ALTER TABLE teams ADD COLUMN IF NOT EXISTS draft_id UUID REFERENCES drafts(id) ON DELETE CASCADE;
CREATE INDEX IF NOT EXISTS teams_draft_idx ON teams (draft_id);

-- Teams drafted before this become the tournament's first draft.
INSERT INTO drafts (tournament_id, name)
SELECT DISTINCT tournament_id, 'Draft 1' FROM teams
WHERE draft_id IS NULL AND tournament_id IS NOT NULL;

INSERT INTO draft_scores (draft_id, player_id, score)
SELECT d.id, p.id, p.skill_score
FROM drafts d JOIN players p ON p.tournament_id = d.tournament_id
WHERE p.skill_score IS NOT NULL AND p.skill_score >= 0
  AND EXISTS (SELECT 1 FROM teams t WHERE t.tournament_id = d.tournament_id AND t.draft_id IS NULL)
ON CONFLICT DO NOTHING;

UPDATE teams t SET draft_id = d.id
FROM drafts d
WHERE t.draft_id IS NULL AND d.tournament_id = t.tournament_id;
