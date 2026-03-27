import { useEffect, useRef, useState } from 'react'
import { cn } from '@/lib/utils'

interface ParticlesProps {
  className?: string
  quantity?: number
  color?: string
  size?: number
  speed?: number
  connectDistance?: number
  vx?: number
  vy?: number
}

interface Particle {
  x: number
  y: number
  vx: number
  vy: number
  size: number
  opacity: number
}

export function Particles({
  className,
  quantity = 60,
  color = '#5b8def',
  size = 2,
  speed = 0.3,
  connectDistance = 120,
}: ParticlesProps) {
  const canvasRef = useRef<HTMLCanvasElement>(null)
  const particles = useRef<Particle[]>([])
  const animationRef = useRef<number>(0)
  const mouse = useRef({ x: -1000, y: -1000 })

  useEffect(() => {
    const canvas = canvasRef.current
    if (!canvas) return
    const ctx = canvas.getContext('2d')
    if (!ctx) return

    const resize = () => {
      const dpr = window.devicePixelRatio || 1
      canvas.width = canvas.offsetWidth * dpr
      canvas.height = canvas.offsetHeight * dpr
      ctx.scale(dpr, dpr)
    }
    resize()
    window.addEventListener('resize', resize)

    const w = () => canvas.offsetWidth
    const h = () => canvas.offsetHeight

    // Init particles
    particles.current = Array.from({ length: quantity }, () => ({
      x: Math.random() * w(),
      y: Math.random() * h(),
      vx: (Math.random() - 0.5) * speed,
      vy: (Math.random() - 0.5) * speed,
      size: size * (0.5 + Math.random() * 0.5),
      opacity: 0.2 + Math.random() * 0.5,
    }))

    const handleMouse = (e: MouseEvent) => {
      const rect = canvas.getBoundingClientRect()
      mouse.current = { x: e.clientX - rect.left, y: e.clientY - rect.top }
    }
    canvas.addEventListener('mousemove', handleMouse)

    const draw = () => {
      ctx.clearRect(0, 0, w(), h())
      const pts = particles.current

      // Update + draw particles
      for (const p of pts) {
        p.x += p.vx
        p.y += p.vy
        if (p.x < 0 || p.x > w()) p.vx *= -1
        if (p.y < 0 || p.y > h()) p.vy *= -1

        ctx.beginPath()
        ctx.arc(p.x, p.y, p.size, 0, Math.PI * 2)
        ctx.fillStyle = color
        ctx.globalAlpha = p.opacity
        ctx.fill()
      }

      // Draw connecting lines
      ctx.globalAlpha = 1
      for (let i = 0; i < pts.length; i++) {
        for (let j = i + 1; j < pts.length; j++) {
          const dx = pts[i].x - pts[j].x
          const dy = pts[i].y - pts[j].y
          const dist = Math.sqrt(dx * dx + dy * dy)
          if (dist < connectDistance) {
            ctx.beginPath()
            ctx.moveTo(pts[i].x, pts[i].y)
            ctx.lineTo(pts[j].x, pts[j].y)
            ctx.strokeStyle = color
            ctx.globalAlpha = 0.08 * (1 - dist / connectDistance)
            ctx.lineWidth = 0.5
            ctx.stroke()
          }
        }

        // Mouse connection
        const mdx = pts[i].x - mouse.current.x
        const mdy = pts[i].y - mouse.current.y
        const mDist = Math.sqrt(mdx * mdx + mdy * mdy)
        if (mDist < connectDistance * 1.5) {
          ctx.beginPath()
          ctx.moveTo(pts[i].x, pts[i].y)
          ctx.lineTo(mouse.current.x, mouse.current.y)
          ctx.strokeStyle = color
          ctx.globalAlpha = 0.15 * (1 - mDist / (connectDistance * 1.5))
          ctx.lineWidth = 0.5
          ctx.stroke()
        }
      }

      ctx.globalAlpha = 1
      animationRef.current = requestAnimationFrame(draw)
    }
    draw()

    return () => {
      cancelAnimationFrame(animationRef.current)
      window.removeEventListener('resize', resize)
      canvas.removeEventListener('mousemove', handleMouse)
    }
  }, [quantity, color, size, speed, connectDistance])

  return (
    <canvas
      ref={canvasRef}
      className={cn('absolute inset-0 h-full w-full pointer-events-auto', className)}
      style={{ pointerEvents: 'none' }}
    />
  )
}
