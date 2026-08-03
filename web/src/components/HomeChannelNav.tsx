import { BookOpenText, Flag, GraduationCap, Music2, Radio, Speech } from 'lucide-react'
import { BiliDynamicIcon, BiliHotIcon } from './BiliIcons'

const channels = [
  '番剧', '电影', '国创', '电视剧', '综艺', '纪录片', '动画', '游戏', '鬼畜', '音乐', '舞蹈', '影视',
  '娱乐', '知识', '科技数码', '资讯', '美食', '小剧场', '汽车', '时尚美妆', '体育运动', '动物',
  'vlog', '绘画', '人工智能', '家装房产', '户外潮流', '更多',
]

const sideChannels = [
  { label: '专栏', icon: BookOpenText },
  { label: '活动', icon: Flag },
  { label: '社区中心', icon: Speech },
  { label: '直播', icon: Radio },
  { label: '课堂', icon: GraduationCap },
  { label: '新歌热榜', icon: Music2 },
]

export function HomeChannelNav() {
  return (
    <nav className="home-channel-nav" aria-label="内容分区">
      <div className="channel-entry-pair">
        <button type="button" className="channel-entry dynamic-entry"><span><BiliDynamicIcon size={24} /></span>动态</button>
        <button type="button" className="channel-entry hot-entry"><span><BiliHotIcon size={25} /></span>热门</button>
      </div>
      <div className="channel-grid">
        {channels.map((channel) => <button type="button" key={channel}>{channel}</button>)}
      </div>
      <div className="channel-side-grid">
        {sideChannels.map(({ label, icon: Icon }) => <button type="button" key={label}><Icon size={16} />{label}</button>)}
      </div>
    </nav>
  )
}
