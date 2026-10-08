import {useClient} from '@/data/DataProvider';
import {EmptyState} from '@/ui/Bits';
import {useRouteParams} from '../common';
import {MovieDetail} from './Movie';
import {AlbumDetail, ArtistDetail} from './Music';
import {AudiobookDetail} from './Audiobook';
import {PhotoAlbumDetail} from './PhotoAlbum';

/** /media/:id — every title that isn't a show: movies, home videos, albums, artists, audiobooks and photo albums. */
export default function MediaScreen() {
  const {itemId} = useRouteParams<{itemId: string}>();
  const client = useClient();
  const id = decodeURIComponent(itemId);
  const entry = client.catalog.entry(id);
  switch (entry?.kind) {
    case 'movie': return <MovieDetail key={id} id={id} />;
    case 'homevideo': return <MovieDetail key={id} id={id} home />;
    case 'album': return <AlbumDetail key={id} id={id} />;
    case 'artist': return <ArtistDetail key={id} id={id} />;
    case 'audiobook': return <AudiobookDetail key={id} id={id} />;
    case 'photoAlbum': return <PhotoAlbumDetail key={id} id={id} />;
    default: return <EmptyState icon="alert" title="This isn’t available" body="It may have been removed from the server, or your access may have changed." actions={[{label: 'Go to Home', to: '/'}]} />;
  }
}
