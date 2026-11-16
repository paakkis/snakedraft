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
instead of counted as zero.

A rank is not taken at face value. It counts for less when there are few
matches behind it or it has not been played for in a long time, and it is
corrected when the player's stats say it is too low. See
[Is the rank right?](#is-the-rank-right) below. Details are in
`backend/services/scoring.go` and `backend/services/rank.go`.

## Drafts and their settings

A tournament can hold any number of drafts. Each one is made with its own
settings and kept next to the others, so you can draft the same players in
different ways and see what changes.

The draft panel has four presets:

| Preset | Scores players on |
| --- | --- |
| Full model | Everything described below. This is what the player list shows. |
| Faceit + Premier | The better of Faceit ELO and Premier rating, as listed |
| Faceit only | Faceit ELO as listed |
| Premier only | Premier rating as listed |

"All settings" opens the rest: which ranks and whether Steam stats are used,
the weight of each component, each of the three rank corrections, the fairness
limit for teammate requests and whether requests are honoured at all. Only the
ratios between the weights matter.

Some settings cannot score everyone. A Faceit-only draft has nothing to go on
for a player who is not on Faceit. By default such a player is scored with the
full model and marked with `~`. Switch that off and they are left out of the
draft instead.

Each draft has a tab with its teams, on the scores it was made with. Those
scores are stored with the draft, so it stays what it was when players are
refreshed later. "Compare all" puts the drafts side by side: their settings,
the spread on their own scores, the spread when the same teams are measured
with the full model, how many teammate requests were kept, and which team
every player is on in each draft.

The code is in `backend/services/settings.go` and `backend/handlers/drafts.go`.

## How the balancing works

Every player ends up with one skill score $s_i$ between 0 and 100. Splitting
$n$ players into $k$ teams of equal size so that the team totals are as close
as possible is the *balanced multiway number partitioning* problem. It is
NP-hard, so the app does not search for the perfect answer. It builds a good
starting point and then improves it with local search. The code is in
`backend/services/balancer.go` and `backend/services/requests.go`.

### 1. The score

The score is a weighted average of normalised components, each clamped to
$[0, 1]$:

$$\text{score} = 100 \cdot \frac{\sum_{c \in P} w_c \, x_c}{\sum_{c \in P} w_c},
\qquad x_c = \operatorname{clamp}\left(\frac{v_c - \min_c}{\max_c - \min_c}\right)$$

$P$ is the set of components the player actually has data for. Dividing by the
weights that are present is what keeps a missing source from counting as zero.

The weights below are the defaults. Each can be changed per draft.

| Component | Weight | Range |
| --- | --- | --- |
| Rank: the better of Faceit ELO and Premier rating | 0.55 × confidence | 0–3000 / 0–35000 |
| Leetify aim | 0.15 | 0–100 |
| Leetify positioning | 0.10 | 0–100 |
| Opening duel win % | 0.08 | 0–100 |
| Recent form | 0.08 | −0.2–0.3 |
| Leetify utility | 0.04 | 0–100 |

Recent form is an exponentially weighted average of the Leetify rating $r_j$
of the last 10 matches, newest first, where Faceit matches count 20% more:

$$\text{form} = \frac{\sum_{j=0}^{9} 0.85^{\,j} \, m_j \, r_j}{\sum_{j=0}^{9} 0.85^{\,j} \, m_j},
\qquad m_j = \begin{cases} 1.2 & \text{Faceit} \\ 1 & \text{otherwise} \end{cases}$$

### Is the rank right?

A new account, a player who rarely plays ranked, or one who has improved a lot
has a rank that says little about their skill. Three checks deal with that.
The code is in `backend/services/rank.go`.

**Confidence.** A rating needs matches to settle. A Faceit account starts at
1000 ELO whatever the player's skill and moves about 25 points per match. With
$n$ matches behind the rank and $t$ years since it was last played for:

$$\text{confidence} = \frac{n}{n + 20} \cdot 0.5^{\,t}$$

That is 0.2 after 5 matches, 0.5 after 20 and 0.9 after 180. The rank's weight
of 0.55 is multiplied by it, so for a thin rank the measured stats decide most
of the score. $n$ comes from Faceit's lifetime stats. For Premier it is
estimated from the share of Premier matches among the 100 latest matches
Leetify lists, applied to the player's total. A part that is unknown is left
out. If the player also has a rank on the other platform, that one backs the
first up: the confidence is at least the other rank's confidence times
$v_\text{other} / v_\text{best}$, the ratio of the two normalised ranks.

**Performance against the lobby.** For the 10 latest Faceit matches the app
looks up the average ELO of the lobby, both teams included. Each match is
rated $r \in [0, 1]$ from K/D (0.6–1.4) and ADR (50–100), where 0.5 is an
average player in that lobby, and implies an ELO:

$$E_j = \text{lobby}_j + 400 \, (2 r_j - 1)$$

An average game means the player belongs in that lobby. A dominant one means
they belong 400 ELO above it. The $E_j$ are averaged with the same 0.85 decay
as recent form, giving $E_\text{perf}$ over $m$ matches. The Faceit ELO is
then moved towards it:

$$E' = E + \left(1 - 0.75\,\frac{n}{n + 20}\right) \frac{m}{m + 5} \cdot 0.5^{\,d / 180} \left(E_\text{perf} - E\right)$$

where $n$ is the number of matches behind the rank and $d$ the days since the
last match. A 5-match account at 1000 ELO that holds its own in 2000 ELO
lobbies is scored as 1425. For a settled rank the same performance moves it by
about a sixth of the gap. This works both ways, so a player who is carried in
lobbies above their level moves down.

The last factor is there because these are the player's latest matches, so
they are exactly as old as the rank. Matches from 20 months ago show the form
the player was in then, and move the rank by a tenth of what fresh ones would.
A player is flagged when $E_\text{perf}$ is at least 150 above their ELO over
5 or more matches, the last of them at most 180 days ago. Faceit has no ADR
for older matches. Those are rated on K/D alone.

This costs up to 11 extra Faceit requests per player. It only works
for Faceit: Leetify does not say what rank the other players in a Premier
match have.

**Mechanics above rank.** Aim and positioning are measured per match and do
not depend on who the player is matched against. Let $M$ be their weighted
average (aim 3, positioning 2) and $R$ the normalised rank, both on a 0–1
scale. The two cannot be compared directly, because mechanics spread over a
much narrower range than ranks do: players ranked around 0.35 still have
mechanics around 0.55. So $M$ is compared with what is typical for the rank:

$$T(R) = 0.40 + 0.45\,R$$

If $M - T(R) > 0.05$:

$$R' = R + 0.5 \, (M - T(R) - 0.05)$$

From a gap of 0.10 the player is flagged as underrated. This only ever raises
a rank. Mechanics below the typical ones are normal for players who win on
game sense and teamwork.

The player list marks these players next to the score: a green ▲ when the
rank looks too low, and an amber ? when it counts for less than half of its
normal weight. Hover over the mark for the numbers.

The constants (20, 400, 0.05 and so on) are judgement calls, checked against a
small pool of real players. They are not fitted to data. The one exception is
$T(R)$, which is a line through the 14 players with a settled rank in one
20-player tournament. That is a thin basis, so treat it as a rough guide. Each
of the three checks can be switched off per draft.

### 2. Snake draft

Players are sorted from best to worst and dealt out in rounds, with the
direction flipping every round. With 4 teams: `1 2 3 4`, then `4 3 2 1`, then
`1 2 3 4` again. The team that gets the best player in a round gets the worst
one in the next, so the totals start out close.

### 3. Local search

The draft is then improved by hill climbing. Two players from different teams
are picked at random and swapped. The swap is kept only if it lowers the
variance of the team averages $\mu_t$:

$$\sigma^2 = \frac{1}{k} \sum_{t=1}^{k} (\mu_t - \bar{\mu})^2$$

This is repeated 10,000 times. Swaps are always one for one, so team sizes
never change. Hill climbing can stop at a local optimum, but starting from a
snake draft it gets very close in practice.

### 4. Teammate requests

Requests are treated as edges in a graph of players. The connected components,
found with a union-find structure, are friend groups that have to stay on the
same team. A group may have at most `team size − 1` members, so nobody can
request a whole team.

Groups are placed first, biggest first, then single players from strongest to
weakest onto the team with the lowest total. After that, a group is swapped
against an equally sized set of players from another team whenever that brings
the two totals closer. For totals $S_x$ and $S_y$ and a swap that moves $d$
points from $y$ to $x$, the change in $S_x^2 + S_y^2$ is

$$\Delta = 2d\,(S_x - S_y + d)$$

and the swap is made while $\Delta < 0$.

A draft is fair when the gap between the strongest and weakest team average is
at most $\max(2.0,\ \text{gap of the draft without requests})$. The 2.0 is
the "max spread" setting of the draft. If it is not
fair, the request whose removal helps the most is dropped, and dropped
requests are retried at the end in case they fit after all.

### Sources

The snake draft has no single known author. It is the usual draft order in
fantasy sports. The rest builds on published work:

- **Number partitioning.** R. L. Graham, "Bounds on multiprocessing timing
  anomalies", *SIAM Journal on Applied Mathematics* 17(2), 1969. The origin of
  the "sort descending, give the next item to the smallest set" greedy rule
  that the snake draft and the group placement are variants of.
  N. Karmarkar and R. M. Karp, "The differencing method of set partitioning",
  UC Berkeley technical report UCB/CSD-83-113, 1982, describes a stronger
  heuristic for the same problem that this app does not use.
  Overview: [Balanced number partitioning](https://en.wikipedia.org/wiki/Balanced_number_partitioning).
- **Local search.** The swap step is plain hill climbing, the same idea as the
  pairwise exchange in B. W. Kernighan and S. Lin, "An efficient heuristic
  procedure for partitioning graphs", *Bell System Technical Journal* 49(2),
  1970. Overview: [Hill climbing](https://en.wikipedia.org/wiki/Hill_climbing).
- **Union-find.** B. A. Galler and M. J. Fischer, "An improved equivalence
  algorithm", *Communications of the ACM* 7(5), 1964.
- **Elo rating.** A. E. Elo, *The Rating of Chessplayers, Past and Present*,
  Arco, 1978. Faceit ELO is based on this system. Valve has not published how
  Premier's CS Rating is calculated. The implied ELO per match follows the
  linear performance rating used in chess, opponents' average plus up to 400.
  Overview: [Performance rating](https://en.wikipedia.org/wiki/Performance_rating_(chess)).
- **Rating confidence.** M. E. Glickman, "Parameter estimation in large
  dynamic paired comparison experiments", *Applied Statistics* 48(3), 1999.
  The Glicko system tracks how uncertain each rating is and lets it grow with
  inactivity. The confidence factor here is a much simpler stand-in for that
  idea, not an implementation of Glicko.
- **Exponential weighting.** S. W. Roberts, "Control chart tests based on
  geometric moving averages", *Technometrics* 1(3), 1959.
