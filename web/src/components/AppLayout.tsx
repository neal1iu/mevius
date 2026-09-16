import { Outlet } from 'react-router-dom'
import Sidebar from '@/components/Sidebar'

function AppLayout() {
  return (
    <div className="flex min-h-svh w-full bg-background">
      <Sidebar />
      <main className="min-w-0 flex-1">
        <div className="mx-auto w-full max-w-5xl px-8 py-10">
          <Outlet />
        </div>
      </main>
    </div>
  )
}

export default AppLayout
