import { useParams } from 'react-router-dom'

function ProjectDetail() {
  const { id } = useParams<{ id: string }>()

  return (
    <div>
      <h1 className="font-heading text-2xl font-medium tracking-tight">
        Project {id}
      </h1>
      <p className="mt-2 text-sm text-muted-foreground">
        Project detail is not implemented yet.
      </p>
    </div>
  )
}

export default ProjectDetail
