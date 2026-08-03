import { ChevronLeft, ChevronRight } from 'lucide-react'
import { useEffect, useState } from 'react'

const carouselModules = import.meta.glob('../assets/carousel/carousel-*', {
  eager: true,
  query: '?url',
  import: 'default',
}) as Record<string, string>

const carouselItems = Object.entries(carouselModules)
  .sort(([left], [right]) => left.localeCompare(right, 'zh-CN'))
  .map(([path, src]) => ({
    src,
    title: carouselTitle(path),
  }))

const loopingSlides = carouselItems.length > 1
  ? [carouselItems[carouselItems.length - 1], ...carouselItems, carouselItems[0]]
  : carouselItems

export function HomeCarousel() {
  const [position, setPosition] = useState(carouselItems.length > 1 ? 1 : 0)
  const [animated, setAnimated] = useState(true)
  const [moving, setMoving] = useState(false)
  const [paused, setPaused] = useState(false)
  const active = carouselItems.length > 1
    ? (position - 1 + carouselItems.length) % carouselItems.length
    : 0

  useEffect(() => {
    if (paused || carouselItems.length < 2) return
    const timer = window.setInterval(() => {
      if (moving) return
      setMoving(true)
      setPosition((current) => current + 1)
    }, 4800)
    return () => window.clearInterval(timer)
  }, [moving, paused])

  function move(offset: number) {
    if (moving || carouselItems.length < 2) return
    setMoving(true)
    setPosition((current) => current + offset)
  }

  function select(index: number) {
    if (moving || index === active) return
    setMoving(true)
    setPosition(index + 1)
  }

  function finishMove() {
    if (position === 0 || position === carouselItems.length + 1) {
      setAnimated(false)
      setPosition(position === 0 ? carouselItems.length : 1)
      window.requestAnimationFrame(() => {
        window.requestAnimationFrame(() => {
          setAnimated(true)
          setMoving(false)
        })
      })
      return
    }
    setMoving(false)
  }

  if (carouselItems.length === 0) return null

  return (
    <section
      className="carousel-root"
      aria-roledescription="轮播图"
      aria-label="首页海报"
      onMouseEnter={() => setPaused(true)}
      onMouseLeave={() => setPaused(false)}
      onFocus={() => setPaused(true)}
      onBlur={() => setPaused(false)}
    >
      <div
        className="carousel-track"
        style={{
          transform: `translateX(-${position * 100}%)`,
          transition: animated ? undefined : 'none',
        }}
        onTransitionEnd={finishMove}
      >
        {loopingSlides.map((item, index) => (
          <figure className="carousel-slide" key={`${item.src}-${index}`} aria-hidden={index !== position}>
            <img src={item.src} alt="" width="780" height="531" fetchPriority={index === (carouselItems.length > 1 ? 1 : 0) ? 'high' : 'auto'} />
          </figure>
        ))}
      </div>
      <div className="carousel-shade" />
      <strong className="carousel-title">{carouselItems[active].title}</strong>
      {carouselItems.length > 1 && <div className="carousel-controls">
        <button type="button" onClick={() => move(-1)} aria-label="上一张海报" title="上一张"><ChevronLeft size={20} /></button>
        <button type="button" onClick={() => move(1)} aria-label="下一张海报" title="下一张"><ChevronRight size={20} /></button>
      </div>}
      <div className="carousel-dots" aria-label="选择海报">
        {carouselItems.map((item, index) => (
          <button
            type="button"
            key={item.src}
            className={active === index ? 'active' : ''}
            aria-label={`第 ${index + 1} 张海报`}
            aria-current={active === index}
            onClick={() => select(index)}
          />
        ))}
      </div>
    </section>
  )
}

function carouselTitle(path: string) {
  const filename = path.split('/').pop() ?? ''
  return filename
    .replace(/^carousel-(?:\d+-)?/, '')
    .replace(/\.[^.]+$/, '')
}
