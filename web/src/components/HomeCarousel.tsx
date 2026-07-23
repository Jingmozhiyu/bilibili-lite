import { ChevronLeft, ChevronRight } from 'lucide-react'
import { useEffect, useState } from 'react'

const posters = [
  { src: '/p1.jpeg', title: '粉色收藏', detail: '把喜欢的事物摆进生活里' },
  { src: '/p2.jpeg', title: '展会现场', detail: '限定硬件与角色设计' },
  { src: '/p3.jpeg', title: '痛车漫游', detail: '花海中的展台记录' },
  { src: '/p4.jpeg', title: '收藏陈列', detail: '一次满载而归的分享' },
  { src: '/p5.jpeg', title: '现场速报', detail: '镜头里的热闹一刻' },
]

export function HomeCarousel() {
  const [active, setActive] = useState(0)
  const [paused, setPaused] = useState(false)

  useEffect(() => {
    if (paused) return
    const timer = window.setInterval(() => setActive((current) => (current + 1) % posters.length), 4800)
    return () => window.clearInterval(timer)
  }, [paused])

  function move(offset: number) {
    setActive((current) => (current + offset + posters.length) % posters.length)
  }

  return (
    <section
      className="poster-carousel"
      aria-roledescription="轮播图"
      aria-label="首页海报"
      onMouseEnter={() => setPaused(true)}
      onMouseLeave={() => setPaused(false)}
      onFocus={() => setPaused(true)}
      onBlur={() => setPaused(false)}
    >
      <div className="poster-track" style={{ transform: `translateX(-${active * 100}%)` }}>
        {posters.map((poster, index) => (
          <figure className="poster-slide" key={poster.src} aria-hidden={index !== active}>
            <img src={poster.src} alt="" fetchPriority={index === 0 ? 'high' : 'auto'} />
          </figure>
        ))}
      </div>
      <div className="poster-shade" />
      <div className="poster-caption">
        <span>站内海报</span>
        <strong>{posters[active].title}</strong>
        <p>{posters[active].detail}</p>
      </div>
      <div className="poster-controls">
        <button type="button" onClick={() => move(-1)} aria-label="上一张海报" title="上一张"><ChevronLeft size={20} /></button>
        <button type="button" onClick={() => move(1)} aria-label="下一张海报" title="下一张"><ChevronRight size={20} /></button>
      </div>
      <div className="poster-dots" aria-label="选择海报">
        {posters.map((poster, index) => (
          <button
            type="button"
            key={poster.src}
            className={active === index ? 'active' : ''}
            aria-label={`第 ${index + 1} 张海报`}
            aria-current={active === index}
            onClick={() => setActive(index)}
          />
        ))}
      </div>
    </section>
  )
}
