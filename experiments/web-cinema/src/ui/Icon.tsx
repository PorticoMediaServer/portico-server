import {
  Activity, AlarmClock, AlignLeft, ArrowDown, ArrowLeft, ArrowRight, ArrowUp, ArrowUpDown, Archive, AudioLines, Bell, BellOff, Book, BookOpen, Bookmark, Captions, Cast, Check,
  CheckCheck, ChevronDown, ChevronLeft, ChevronRight, ChevronUp, CircleAlert, CircleCheck, CircleHelp, CircleUser, Clapperboard, Clock, Cloud, Copy, Cpu, Database, Disc3, Download,
  Ellipsis, ExternalLink, Eye, EyeOff, FileText, Film, Filter, Flag, Folder, FolderOpen, Gauge, Globe, GripVertical, HardDrive, Heart, History, House, Image, Images, Info, KeyRound,
  Keyboard, Languages, LayoutGrid, Library, Link2, List, ListMusic, ListPlus, ListVideo, Lock, LogOut, Mail, Maximize, Menu, MessageSquare, Mic2, Minimize2, Minus, Monitor, MonitorPlay,
  Moon, Music, Network, Palette, Pause, PanelLeftClose, PanelLeftOpen, Pencil, PictureInPicture2, Pin, Play, Plus, Podcast, QrCode, Radio, RefreshCw, Repeat, Repeat1, RotateCcw, RotateCw,
  Save, ScrollText, Search, Send, Server, Settings, Settings2, Share2, Shield, ShieldCheck, Shuffle, SkipBack, SkipForward, SlidersHorizontal, Smartphone, Sparkles, Square, Star,
  StarHalf, Sun, Tablet, Tag, Tags, ThumbsDown, ThumbsUp, Timer, Trash2, TrendingUp, Tv, TvMinimalPlay, Undo2, Upload, User, UserPlus, Users, Video, Volume2, VolumeX, Wand2, Wifi,
  WifiOff, Wrench, X, Zap, Calendar, CircleDot, Layers, Workflow, Waves, Hash, Lightbulb, Airplay,
  type LucideIcon,
} from 'lucide-react';

const icons = {
  activity: Activity, alarm: AlarmClock, about: AlignLeft, down: ArrowDown, back: ArrowLeft, forward: ArrowRight, up: ArrowUp, sort: ArrowUpDown, archive: Archive, audio: AudioLines,
  bell: Bell, bellOff: BellOff, book: Book, bookOpen: BookOpen, bookmark: Bookmark, captions: Captions, cast: Cast, check: Check, checkAll: CheckCheck, chevronDown: ChevronDown,
  chevronLeft: ChevronLeft, chevronRight: ChevronRight, chevronUp: ChevronUp, alert: CircleAlert, success: CircleCheck, help: CircleHelp, account: CircleUser, movie: Clapperboard,
  clock: Clock, cloud: Cloud, copy: Copy, cpu: Cpu, database: Database, disc: Disc3, download: Download, more: Ellipsis, external: ExternalLink, eye: Eye, eyeOff: EyeOff, file: FileText,
  film: Film, filter: Filter, flag: Flag, folder: Folder, folderOpen: FolderOpen, gauge: Gauge, globe: Globe, grip: GripVertical, drive: HardDrive, heart: Heart, history: History,
  home: House, image: Image, photos: Images, info: Info, key: KeyRound, keyboard: Keyboard, languages: Languages, grid: LayoutGrid, library: Library, link: Link2, list: List,
  playlist: ListMusic, listPlus: ListPlus, queue: ListVideo, lock: Lock, signOut: LogOut, mail: Mail, fullscreen: Maximize, menu: Menu, message: MessageSquare, mic: Mic2,
  minimize: Minimize2, minus: Minus, monitor: Monitor, trailer: MonitorPlay, moon: Moon, music: Music, network: Network, palette: Palette, pause: Pause, collapse: PanelLeftClose,
  expand: PanelLeftOpen, edit: Pencil, pip: PictureInPicture2, pin: Pin, play: Play, plus: Plus, podcast: Podcast, qr: QrCode, live: Radio, refresh: RefreshCw, repeat: Repeat,
  repeatOne: Repeat1, rewind: RotateCcw, fastForward: RotateCw, save: Save, logs: ScrollText, search: Search, send: Send, server: Server, settings: Settings, tune: Settings2,
  share: Share2, shield: Shield, shieldCheck: ShieldCheck, shuffle: Shuffle, previous: SkipBack, next: SkipForward, sliders: SlidersHorizontal, phone: Smartphone, sparkles: Sparkles,
  stop: Square, star: Star, starHalf: StarHalf, sun: Sun, tablet: Tablet, tag: Tag, tags: Tags, dislike: ThumbsDown, like: ThumbsUp, timer: Timer, trash: Trash2, trending: TrendingUp,
  tv: Tv, channels: TvMinimalPlay, undo: Undo2, upload: Upload, profile: User, addPerson: UserPlus, people: Users, video: Video, volume: Volume2, mute: VolumeX, wand: Wand2,
  wifi: Wifi, wifiOff: WifiOff, wrench: Wrench, close: X, zap: Zap, calendar: Calendar, record: CircleDot, layers: Layers, workflow: Workflow, waves: Waves, hash: Hash, idea: Lightbulb,
  airplay: Airplay,
} satisfies Record<string, LucideIcon>;

export type IconName = keyof typeof icons;

export function Icon({name, size = 20, stroke = 1.9, className, fill, style}: {name: IconName; size?: number; stroke?: number; className?: string; fill?: string; style?: React.CSSProperties}) {
  const C = icons[name];
  return <C size={size} strokeWidth={stroke} className={className} aria-hidden focusable={false} fill={fill ?? 'none'} style={style} />;
}
