import { jobs, runtime } from '@/mocks/db'
import type { Job } from '@/api/types'

export function createJob(type: string, title: string, resourceId?: string): Job {
  const job: Job = {
    id: `job-${++runtime.jobCounter}`,
    type,
    title,
    resourceId,
    state: 'queued',
    progress: null,
    createdAt: new Date().toISOString(),
  }
  jobs.unshift(job)
  return job
}
