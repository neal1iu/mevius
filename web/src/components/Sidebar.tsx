import { NavLink } from 'react-router-dom'
import { cn } from '@/lib/utils'

const navItems = [
  { to: '/projects', label: 'Projects' },
  { to: '/inventory', label: 'Inventory' },
  { to: '/accounts', label: 'Connections' },
]

function Sidebar() {
  return (
    <aside className="flex w-60 shrink-0 flex-col border-r border-border bg-sidebar">
      <div className="flex h-16 items-center gap-2.5 border-b border-border px-6">
        <div className="flex size-6 items-center justify-center rounded-md bg-foreground text-[11px] font-semibold tracking-tight text-background">
          m
        </div>
        <span className="font-heading text-sm font-medium tracking-tight">
          mevius
        </span>
      </div>
      <nav className="flex flex-col gap-1 px-3 py-4">
        {navItems.map((item) => (
          <NavLink
            key={item.to}
            to={item.to}
            className={({ isActive }) =>
              cn(
                'rounded-md px-3 py-2 text-sm font-medium text-muted-foreground transition-colors hover:bg-muted hover:text-foreground',
                isActive && 'bg-muted text-foreground',
              )
            }
          >
            {item.label}
          </NavLink>
        ))}
      </nav>
    </aside>
  )
}

export default Sidebar
