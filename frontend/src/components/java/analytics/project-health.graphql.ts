import { gql } from "@apollo/client";

export const PROJECT_HEALTH = gql`
  query ProjectHealth($projectId: ID!) {
    projectHealth(projectId: $projectId) {
      stats {
        taskCountByStatus { todo inProgress done }
        taskCountByPriority { low medium high }
        overdueCount
        avgCompletionTimeHours
        memberWorkload { userId name assignedCount completedCount }
      }
      velocity {
        weeklyThroughput { week completed created }
        avgLeadTimeHours
        leadTimePercentiles { p50 p75 p95 }
      }
      activity {
        totalEvents
        eventCountByType { eventType count }
        commentCount
        activeContributors
        weeklyActivity { week events comments }
      }
    }
  }
`;

export interface TaskStatusCounts { todo: number; inProgress: number; done: number; }
export interface TaskPriorityCounts { low: number; medium: number; high: number; }
export interface MemberWorkload {
  userId: string;
  name: string;
  assignedCount: number;
  completedCount: number;
}
export interface ProjectStats {
  taskCountByStatus: TaskStatusCounts;
  taskCountByPriority: TaskPriorityCounts;
  overdueCount: number;
  avgCompletionTimeHours: number | null;
  memberWorkload: MemberWorkload[];
}
export interface WeeklyThroughput { week: string; completed: number; created: number; }
export interface Percentiles { p50: number; p75: number; p95: number; }
export interface VelocityMetrics {
  weeklyThroughput: WeeklyThroughput[];
  avgLeadTimeHours: number | null;
  leadTimePercentiles: Percentiles;
}
export interface EventTypeCount { eventType: string; count: number; }
export interface WeeklyActivity { week: string; events: number; comments: number; }
export interface ActivityStats {
  totalEvents: number;
  eventCountByType: EventTypeCount[];
  commentCount: number;
  activeContributors: number;
  weeklyActivity: WeeklyActivity[];
}
export interface ProjectHealth {
  stats: ProjectStats;
  velocity: VelocityMetrics;
  activity: ActivityStats;
}
export interface ProjectHealthData {
  projectHealth: ProjectHealth;
}
