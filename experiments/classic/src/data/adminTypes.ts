import type {Img} from './types';

/**
 * Server administration is described as pages of typed blocks. The server
 * projects each section into these blocks; the app renders them the same way
 * on every form factor, which keeps 20+ sections consistent and lets the TV
 * show them read-mostly without separate code.
 */
export type Tone = 'neutral' | 'good' | 'warn' | 'bad' | 'accent';
export type AdminAction = {id: string; label: string; icon?: string; tone?: 'primary' | 'secondary' | 'danger' | 'ghost'; confirm?: {title: string; body: string; action: string; typeToConfirm?: string}; form?: AdminForm; target?: string; navigate?: {section: string; param?: string}};
export type AdminForm = {title: string; lede?: string; submit: string; fields: AdminField[]};
export type AdminField =
  | {kind: 'text'; key: string; label: string; help?: string; placeholder?: string; value?: string; secure?: boolean; multiline?: boolean}
  | {kind: 'number'; key: string; label: string; help?: string; value?: number; unit?: string; min?: number; max?: number; step?: number}
  | {kind: 'toggle'; key: string; label: string; help?: string; value?: boolean}
  | {kind: 'choice'; key: string; label: string; help?: string; value?: string; options: {value: string; label: string; help?: string}[]}
  | {kind: 'multi'; key: string; label: string; help?: string; value?: string[]; options: {value: string; label: string}[]}
  | {kind: 'folder'; key: string; label: string; help?: string; value?: string; placeholder?: string};

export type FolderEntry = {name: string; path: string; access: boolean; link?: boolean};
export type FolderListing = {path: string; parent?: string; entries: FolderEntry[]; roots: FolderEntry[]};

export type ListItem = {id: string; title: string; subtitle?: string; meta?: string; icon?: string; image?: Img; round?: boolean; badge?: {label: string; tone: Tone}; progress?: number; actions?: AdminAction[]; open?: {section: string; param?: string}; detail?: {label: string; value: string}[]};

export type Block =
  | {type: 'stats'; items: {label: string; value: string; sub?: string; tone?: Tone; spark?: number[]}[]}
  | {type: 'notice'; tone: Tone; title: string; body: string; actions?: AdminAction[]}
  | {type: 'settings'; title?: string; lede?: string; fields: AdminField[]}
  | {type: 'facts'; title?: string; lede?: string; rows: {label: string; value: string; tone?: Tone; copy?: boolean}[]}
  | {type: 'list'; title?: string; lede?: string; items: ListItem[]; empty?: {title: string; body: string}; actions?: AdminAction[]; filters?: {key: string; options: {value: string; label: string}[]; value: string}}
  | {type: 'table'; title?: string; lede?: string; columns: string[]; rows: string[][]}
  | {type: 'chart'; title: string; lede?: string; series: {label: string; values: number[]; tone?: Tone}[]; unit?: string; labels?: string[]}
  | {type: 'meters'; title?: string; items: {label: string; value: number; detail: string; tone?: Tone}[]}
  | {type: 'actions'; title?: string; lede?: string; actions: AdminAction[]}
  | {type: 'timeline'; title: string; lede?: string; lanes: {label: string; spans: {start: number; end: number; label: string; tone: Tone}[]}[]; from: number; to: number};

export type AdminPage = {title: string; lede: string; blocks: Block[]; actions?: AdminAction[]; back?: {section: string; label: string}};
export type AdminSectionMeta = {id: string; label: string; group: 'Console' | 'Media' | 'Live TV' | 'Server'; icon: string; lede: string};
