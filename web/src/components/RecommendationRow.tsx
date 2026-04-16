import { Recommendation } from '../api/client'
import RecommendationCard from './RecommendationCard'

interface RecommendationRowProps {
  title: string
  recommendations: Recommendation[]
  onDismiss: (id: number) => void
  onAdd: (id: number) => void
  onExcludeAuthor: (authorName: string) => void
}

export default function RecommendationRow({ title, recommendations, onDismiss, onAdd, onExcludeAuthor }: RecommendationRowProps) {
  if (recommendations.length === 0) return null

  return (
    <div className="mb-6">
      <div className="flex items-center justify-between mb-3">
        <h3 className="text-lg font-semibold">{title}</h3>
      </div>
      <div className="flex gap-3 overflow-x-auto pb-2 scrollbar-thin">
        {recommendations.map(rec => (
          <RecommendationCard
            key={rec.id}
            rec={rec}
            onDismiss={onDismiss}
            onAdd={onAdd}
            onExcludeAuthor={onExcludeAuthor}
          />
        ))}
      </div>
    </div>
  )
}
