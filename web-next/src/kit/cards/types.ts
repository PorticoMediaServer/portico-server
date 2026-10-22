import type {CSSProperties, FocusEventHandler, ReactNode, Ref} from 'react';
import type {CardTextModel, FactPart} from '@portico/client-core/kit-model';
import type {ArtworkSource, AmbientArtSource} from '../artwork';

export type CardContent = Pick<CardTextModel, 'title' | 'caption'> & Partial<Pick<CardTextModel, 'detail' | 'progress'>>;
export type CardSelectionIntent = Readonly<{range: boolean; source: 'pointer' | 'keyboard' | 'long-press'}>;
/** The shared selection family supplies the actual SelectionCircle; cards positions it. */
export type CardSelection = Readonly<{
  active: boolean;
  checked: boolean;
  control: ReactNode;
  onToggle: (intent: CardSelectionIntent) => void;
  onExit: () => void;
}>;
export type CardProps = Readonly<{
  model: CardContent;
  onNavigate: () => void;
  onPlay?: () => void;
  playLabel?: string;
  onMore?: (anchor: HTMLButtonElement) => void;
  moreExpanded?: boolean;
  onNotInterested?: () => void;
  selection?: CardSelection;
  watched?: boolean;
  isNew?: boolean;
  next?: boolean;
  className?: string;
  style?: CSSProperties;
  primaryRef?: Ref<HTMLButtonElement>;
  onFocus?: FocusEventHandler<HTMLButtonElement>;
  onBlur?: FocusEventHandler<HTMLButtonElement>;
  visible?: boolean;
  context?: 'shelf' | 'grid' | 'search' | 'detail';
  density?: 'shelf' | 'grid';
}>;
export type MediaCardProps = CardProps & Readonly<{
  artwork?: ArtworkSource;
  arrivalDurationMs?: number;
  fallback?: ReactNode;
  position?: string;
  ambient?: AmbientArtSource;
  palette?: Readonly<{first: string; second: string}>;
}>;
export type CardFacts = readonly FactPart[];
