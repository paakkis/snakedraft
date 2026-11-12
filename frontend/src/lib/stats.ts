import type { Team } from '../types'

const averages = (teams: Team[]) => teams.map(t => t.avg_skill_score)

/** Variance of team averages — same metric the backend optimises. */
export function teamVariance(teams: Team[]): number {
  const avgs = averages(teams)
  if (avgs.length === 0) return 0
  const mean = avgs.reduce((a, b) => a + b, 0) / avgs.length
  return avgs.reduce((sum, a) => sum + (a - mean) ** 2, 0) / avgs.length
}

/** Gap between the strongest and weakest team average. */
export function teamSpread(teams: Team[]): number {
  const avgs = averages(teams)
  if (avgs.length === 0) return 0
  return Math.max(...avgs) - Math.min(...avgs)
}

export function teamMean(teams: Team[]): number {
  const avgs = averages(teams)
  return avgs.length ? avgs.reduce((a, b) => a + b, 0) / avgs.length : 0
}
