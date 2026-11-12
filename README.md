# snakedraft

Makes balanced teams for CS2 tournaments among friends. Add players by Steam
profile, it pulls their stats from Leetify, Faceit and Steam, gives everyone a
skill score and drafts even teams. Players can ask to be on the same team as a
friend, and that is respected as long as the teams stay fair.

## Running it

You need Docker. Put your API keys in a `.env` file in the repo root:

```
STEAM_API_KEY=
LEETIFY_API_KEY=
FACEIT_API_KEY=
```

The Steam key is required. The other two are optional but the scores are much
better with them.

```
docker compose up --build
```

Then open http://localhost:5173.

## Tests

```
cd backend && go test ./...
```

## How the score works

Mostly rank (the better of Faceit ELO and Premier rating) plus Leetify's aim,
positioning and utility ratings and recent form. Steam lifetime stats are only
a small part, unless a player has nothing else. Missing data is left out
instead of counted as zero. Details are in `backend/services/scoring.go`.
