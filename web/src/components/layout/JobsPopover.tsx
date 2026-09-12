import { useNavigate } from 'react-router-dom'
import { ListTodo } from 'lucide-react'
import { useJobs } from '@/api/queries'
import { JobProgress } from '@/components/core/job-progress'
import { Button } from '@/components/ui/button'
import { Popover, PopoverContent, PopoverTrigger } from '@/components/ui/popover'
import { ScrollArea } from '@/components/ui/scroll-area'

const ACTIVE_STATES = ['queued', 'preparing', 'running']

export function JobsPopover() {
  const navigate = useNavigate()
  const { data: jobs } = useJobs()
  const active = (jobs ?? []).filter((j) => ACTIVE_STATES.includes(j.state))
  const recent = (jobs ?? [])
    .filter((j) => !ACTIVE_STATES.includes(j.state))
    .slice(0, 3)

  return (
    <Popover>
      <PopoverTrigger asChild>
        <Button variant="ghost" size="icon-sm" aria-label="Jobs" className="relative">
          <ListTodo className="size-4" />
          {active.length > 0 && (
            <span className="tnum absolute -top-1 -right-1 flex size-4 items-center justify-center rounded-full bg-primary text-[10px] font-semibold text-primary-foreground">
              {active.length}
            </span>
          )}
        </Button>
      </PopoverTrigger>
      <PopoverContent align="end" className="w-96 p-0">
        <div className="flex items-center justify-between border-b px-4 py-3">
          <p className="text-sm font-semibold">Jobs</p>
          <span className="text-xs text-muted-foreground">
            {active.length > 0 ? `${active.length} running` : 'All idle'}
          </span>
        </div>
        <ScrollArea className="max-h-96">
          <div className="flex flex-col gap-4 p-4">
            {active.length === 0 && recent.length === 0 ? (
              <p className="py-4 text-center text-sm text-muted-foreground">No recent jobs.</p>
            ) : (
              <>
                {active.map((job) => (
                  <JobProgress key={job.id} job={job} />
                ))}
                {recent.map((job) => (
                  <JobProgress key={job.id} job={job} />
                ))}
              </>
            )}
          </div>
        </ScrollArea>
        <div className="border-t p-2">
          <Button
            variant="ghost"
            size="sm"
            className="w-full text-xs text-muted-foreground"
            onClick={() => navigate('/monitoring?tab=jobs')}
          >
            View all activity
          </Button>
        </div>
      </PopoverContent>
    </Popover>
  )
}
