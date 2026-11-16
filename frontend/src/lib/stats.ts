import type { Team } from '../types'

const mean = (values: number[]) => (values.length ? values.reduce((a, b) => a + b, 0) / values.length : 0)

const averages = (teams: Team[]) => teams.map(t => t.avg_skill_score)

/** Average score of a team's players, ignoring those without one. */
export const averageScore = (scores: (number | null | undefined)[]) =>
  mean(scores.filter((s): s is number => s != null && s >= 0))

/** Variance of the given team averages - same metric the backend optimises. */
export function variance(avgs: number[]): number {
  const m = mean(avgs)
  return avgs.length ? avgs.reduce((sum, a) => sum + (a - m) ** 2, 0) / avgs.length : 0
}

/** Gap between the highest and lowest of the given team averages. */
export const spread = (avgs: number[]) => (avgs.length ? Math.max(...avgs) - Math.min(...avgs) : 0)

export const teamVariance = (teams: Team[]) => variance(averages(teams))

/** Gap between the strongest and weakest team average. */
export const teamSpread = (teams: Team[]) => spread(averages(teams))

export const teamMean = (teams: Team[]) => mean(averages(teams))
