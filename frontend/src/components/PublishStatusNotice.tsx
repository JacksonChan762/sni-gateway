import { Link } from 'react-router-dom'
import { useQuery } from '@tanstack/react-query'
import { AlertCircle, ArrowRight } from 'lucide-react'
import { getApiError, getPublishStatus } from '../api/client'

export default function PublishStatusNotice() {
  const { data, error, refetch } = useQuery({
    queryKey: ['publish-status'],
    queryFn: getPublishStatus,
    refetchOnMount: 'always',
  })

  if (error) {
    return <div className="card border-yellow-800/60 text-sm text-yellow-200" role="alert">
      <div className="flex items-start gap-2">
        <AlertCircle size={17} className="mt-0.5 flex-shrink-0" />
        <div>
          <p>Could not check whether saved routes are published: {getApiError(error)}</p>
          <button className="underline mt-1" onClick={() => void refetch()}>Try again</button>
        </div>
      </div>
    </div>
  }
  if (!data?.pending) return null

  return <div className="card border-amber-700/50 bg-amber-950/20 flex flex-wrap items-center justify-between gap-3" role="status">
    <div className="flex items-start gap-3 min-w-0">
      <AlertCircle size={18} className="text-amber-300 mt-0.5 flex-shrink-0" />
      <div>
        <p className="text-sm font-medium text-amber-100">Saved routes are waiting to be published</p>
        <p className="text-xs text-amber-200/70 mt-1">These saved changes are not in the nginx configuration yet.</p>
      </div>
    </div>
    <Link to="/publish" className="btn-primary text-xs">Review and publish <ArrowRight size={13} /></Link>
  </div>
}
