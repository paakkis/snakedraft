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

| Component | Weight | Range |
| --- | --- | --- |
| Rank: the better of Faceit ELO and Premier rating | 0.55 | 0–3000 / 0–35000 |
| Leetify aim | 0.15 | 0–100 |
| Leetify positioning | 0.10 | 0–100 |
| Opening duel win % | 0.08 | 0–100 |
| Recent form | 0.08 | −0.2–0.3 |
| Leetify utility | 0.04 | 0–100 |

Recent form is an exponentially weighted average of the Leetify rating $r_j$
of the last 10 matches, newest first, where Faceit matches count 20% more:

$$\text{form} = \frac{\sum_{j=0}^{9} 0.85^{\,j} \, m_j \, r_j}{\sum_{j=0}^{9} 0.85^{\,j} \, m_j},
\qquad m_j = \begin{cases} 1.2 & \text{Faceit} \\ 1 & \text{otherwise} \end{cases}$$

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
at most $\max(2.0,\ \text{gap of the draft without requests})$. If it is not
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
  Premier's CS Rating is calculated.
- **Exponential weighting.** S. W. Roberts, "Control chart tests based on
  geometric moving averages", *Technometrics* 1(3), 1959.
