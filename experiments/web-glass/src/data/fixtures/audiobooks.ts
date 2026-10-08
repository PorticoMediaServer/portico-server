import type { Audiobook, AudiobookChapter } from '../types.ts';
import { art, daysAgo, paletteFor, rng } from './util.ts';

export interface AudiobookDef {
  id: string;
  title: string;
  author: string;
  narrator: string;
  year: number;
  duration: string; // h:mm
  genres: string[];
  summary: string;
  cover: number;
  coverStyle: 'classic' | 'modern' | 'kids' | 'gothic';
  chapters: string[];
  added: number;
  series?: { name: string; index: number };
  opening: string; // public-domain opening text, narrated for the fixture stream
  voice: string;
}

const roman = (n: number) => {
  const map: [number, string][] = [[50, 'L'], [40, 'XL'], [10, 'X'], [9, 'IX'], [5, 'V'], [4, 'IV'], [1, 'I']];
  let out = '';
  for (const [v, s] of map) while (n >= v) { out += s; n -= v; }
  return out;
};
const numbered = (n: number, prefix = 'Chapter', r = false) => Array.from({ length: n }, (_, i) => `${prefix} ${r ? roman(i + 1) : i + 1}`);

export const audiobookDefs: AudiobookDef[] = [
  {
    id: 'pride-and-prejudice',
    title: 'Pride and Prejudice',
    author: 'Jane Austen',
    narrator: 'Eleanor Whitby',
    year: 1813,
    duration: '11:35',
    genres: ['Classics', 'Romance'],
    summary: 'When the wealthy and aloof Mr. Darcy arrives in Hertfordshire, the quick-witted Elizabeth Bennet takes an instant dislike to him — and he to her. Austen’s most beloved novel is a comedy of manners, money and first impressions.',
    cover: 19,
    coverStyle: 'classic',
    chapters: numbered(61),
    added: 30,
    voice: 'Kate',
    opening:
      'Chapter One. It is a truth universally acknowledged, that a single man in possession of a good fortune, must be in want of a wife. However little known the feelings or views of such a man may be on his first entering a neighbourhood, this truth is so well fixed in the minds of the surrounding families, that he is considered the rightful property of some one or other of their daughters. "My dear Mr. Bennet," said his lady to him one day, "have you heard that Netherfield Park is let at last?" Mr. Bennet replied that he had not. "But it is," returned she; "for Mrs. Long has just been here, and she told me all about it." Mr. Bennet made no answer. "Do you not want to know who has taken it?" cried his wife impatiently. "You want to tell me, and I have no objection to hearing it." This was invitation enough.',
  },
  {
    id: 'frankenstein',
    title: 'Frankenstein',
    author: 'Mary Shelley',
    narrator: 'Thomas Aldridge',
    year: 1818,
    duration: '8:35',
    genres: ['Classics', 'Horror', 'Science Fiction'],
    summary: 'Obsessed with discovering the secret of life, young scientist Victor Frankenstein builds a man from the dead — and then abandons him. Shelley’s masterpiece is the first great science-fiction novel, and still one of the most unsettling.',
    cover: 394,
    coverStyle: 'gothic',
    chapters: [...numbered(4, 'Letter'), ...numbered(24)],
    added: 80,
    voice: 'Daniel',
    opening:
      'Letter One. To Mrs. Saville, England. St. Petersburgh, December the eleventh. You will rejoice to hear that no disaster has accompanied the commencement of an enterprise which you have regarded with such evil forebodings. I arrived here yesterday, and my first task is to assure my dear sister of my welfare and increasing confidence in the success of my undertaking. I am already far north of London, and as I walk in the streets of Petersburgh, I feel a cold northern breeze play upon my cheeks, which braces my nerves and fills me with delight.',
  },
  {
    id: 'the-adventures-of-sherlock-holmes',
    title: 'The Adventures of Sherlock Holmes',
    author: 'Arthur Conan Doyle',
    narrator: 'Thomas Aldridge',
    year: 1892,
    duration: '10:25',
    genres: ['Classics', 'Mystery'],
    series: { name: 'Sherlock Holmes', index: 3 },
    summary: 'Twelve cases for the world’s only consulting detective — from the matter of Irene Adler to the peculiar affair of the Red-Headed League.',
    cover: 49,
    coverStyle: 'classic',
    chapters: [
      'A Scandal in Bohemia',
      'The Red-Headed League',
      'A Case of Identity',
      'The Boscombe Valley Mystery',
      'The Five Orange Pips',
      'The Man with the Twisted Lip',
      'The Adventure of the Blue Carbuncle',
      'The Adventure of the Speckled Band',
      'The Adventure of the Engineer’s Thumb',
      'The Adventure of the Noble Bachelor',
      'The Adventure of the Beryl Coronet',
      'The Adventure of the Copper Beeches',
    ],
    added: 12,
    voice: 'Daniel',
    opening:
      'A Scandal in Bohemia. To Sherlock Holmes she is always the woman. I have seldom heard him mention her under any other name. In his eyes she eclipses and predominates the whole of her sex. It was not that he felt any emotion akin to love for Irene Adler. All emotions, and that one particularly, were abhorrent to his cold, precise but admirably balanced mind. He was, I take it, the most perfect reasoning and observing machine that the world has seen.',
  },
  {
    id: 'moby-dick',
    title: 'Moby-Dick',
    author: 'Herman Melville',
    narrator: 'James Harlan',
    year: 1851,
    duration: '21:05',
    genres: ['Classics', 'Adventure'],
    summary: 'Ishmael signs on to the whaler Pequod and discovers that its captain, Ahab, has only one purpose: to hunt down the white whale that took his leg.',
    cover: 157,
    coverStyle: 'modern',
    chapters: [...numbered(135), 'Epilogue'],
    added: 200,
    voice: 'Alex',
    opening:
      'Chapter One. Loomings. Call me Ishmael. Some years ago, never mind how long precisely, having little or no money in my purse, and nothing particular to interest me on shore, I thought I would sail about a little and see the watery part of the world. It is a way I have of driving off the spleen and regulating the circulation. Whenever I find myself growing grim about the mouth; whenever it is a damp, drizzly November in my soul; then, I account it high time to get to sea as soon as I can.',
  },
  {
    id: 'the-time-machine',
    title: 'The Time Machine',
    author: 'H. G. Wells',
    narrator: 'James Harlan',
    year: 1895,
    duration: '3:25',
    genres: ['Classics', 'Science Fiction'],
    summary: 'A Victorian inventor travels to the year 802,701 and finds humanity divided into two strange species — the gentle Eloi and the subterranean Morlocks.',
    cover: 456,
    coverStyle: 'modern',
    chapters: [...numbered(12, 'Chapter', true), 'Epilogue'],
    added: 4,
    voice: 'Daniel',
    opening:
      'Chapter One. The Time Traveller, for so it will be convenient to speak of him, was expounding a recondite matter to us. His grey eyes shone and twinkled, and his usually pale face was flushed and animated. The fire burned brightly, and the soft radiance of the incandescent lights in the lilies of silver caught the bubbles that flashed and passed in our glasses.',
  },
  {
    id: 'little-women',
    title: 'Little Women',
    author: 'Louisa May Alcott',
    narrator: 'Eleanor Whitby',
    year: 1868,
    duration: '18:50',
    genres: ['Classics', 'Family'],
    summary: 'Meg, Jo, Beth and Amy March grow up in Civil War–era Massachusetts, with little money, a great deal of spirit and a mother who holds everything together.',
    cover: 82,
    coverStyle: 'classic',
    chapters: numbered(47),
    added: 150,
    voice: 'Samantha',
    opening:
      'Chapter One. Playing Pilgrims. "Christmas won\'t be Christmas without any presents," grumbled Jo, lying on the rug. "It\'s so dreadful to be poor!" sighed Meg, looking down at her old dress. "I don\'t think it\'s fair for some girls to have plenty of pretty things, and other girls nothing at all," added little Amy, with an injured sniff. "We\'ve got Father and Mother, and each other," said Beth contentedly from her corner.',
  },
  {
    id: 'dracula',
    title: 'Dracula',
    author: 'Bram Stoker',
    narrator: 'Thomas Aldridge',
    year: 1897,
    duration: '15:30',
    genres: ['Classics', 'Horror'],
    summary: 'Solicitor Jonathan Harker travels to Transylvania to help a nobleman buy a house in England. By the time he realizes what his client is, the Count is already on his way.',
    cover: 599,
    coverStyle: 'gothic',
    chapters: numbered(27, 'Chapter', true),
    added: 20,
    voice: 'Daniel',
    opening:
      'Chapter One. Jonathan Harker\'s Journal. Third of May. Bistritz. Left Munich at 8:35 P.M., on first May, arriving at Vienna early next morning; should have arrived at 6:46, but train was an hour late. Buda-Pesth seems a wonderful place, from the glimpse which I got of it from the train and the little I could walk through the streets. I feared to go very far from the station, as we had arrived late and would start as near the correct time as possible.',
  },
  {
    id: 'the-wind-in-the-willows',
    title: 'The Wind in the Willows',
    author: 'Kenneth Grahame',
    narrator: 'Eleanor Whitby',
    year: 1908,
    duration: '5:50',
    genres: ['Classics', 'Kids'],
    summary: 'Mole, Ratty, Badger and the incorrigible Mr. Toad mess about in boats, get lost in the Wild Wood and take back Toad Hall from the weasels.',
    cover: 60,
    coverStyle: 'kids',
    chapters: [
      'The River Bank',
      'The Open Road',
      'The Wild Wood',
      'Mr. Badger',
      'Dulce Domum',
      'Mr. Toad',
      'The Piper at the Gates of Dawn',
      'Toad’s Adventures',
      'Wayfarers All',
      'The Further Adventures of Toad',
      '“Like Summer Tempests Came His Tears”',
      'The Return of Ulysses',
    ],
    added: 45,
    voice: 'Kate',
    opening:
      'The River Bank. The Mole had been working very hard all the morning, spring-cleaning his little home. First with brooms, then with dusters; then on ladders and steps and chairs, with a brush and a pail of whitewash; till he had dust in his throat and eyes, and splashes of whitewash all over his black fur, and an aching back and weary arms. Spring was moving in the air above and in the earth below and around him.',
  },
  {
    id: 'meditations',
    title: 'Meditations',
    author: 'Marcus Aurelius',
    narrator: 'James Harlan',
    year: 180,
    duration: '5:10',
    genres: ['Philosophy', 'Classics'],
    summary: 'The private notebook of a Roman emperor, never meant for publication: twelve books of reminders to himself about duty, mortality and how to live well.',
    cover: 266,
    coverStyle: 'modern',
    chapters: ['Book One', 'Book Two', 'Book Three', 'Book Four', 'Book Five', 'Book Six', 'Book Seven', 'Book Eight', 'Book Nine', 'Book Ten', 'Book Eleven', 'Book Twelve'],
    added: 7,
    voice: 'Alex',
    opening:
      'Book One. From my grandfather Verus I learned good morals and the government of my temper. From the reputation and remembrance of my father, modesty and a manly character. From my mother, piety and beneficence, and abstinence, not only from evil deeds, but even from evil thoughts; and further, simplicity in my way of living, far removed from the habits of the rich.',
  },
  {
    id: 'around-the-world-in-eighty-days',
    title: 'Around the World in Eighty Days',
    author: 'Jules Verne',
    narrator: 'James Harlan',
    year: 1872,
    duration: '7:05',
    genres: ['Classics', 'Adventure'],
    summary: 'On a wager at the Reform Club, the precise Phileas Fogg sets out to circle the globe in eighty days — with his new valet Passepartout and a detective convinced Fogg is a bank robber.',
    cover: 174,
    coverStyle: 'classic',
    chapters: numbered(37, 'Chapter', true),
    added: 100,
    voice: 'Alex',
    opening:
      'Chapter One. Mr. Phileas Fogg lived, in 1872, at No. 7, Saville Row, Burlington Gardens, the house in which Sheridan died in 1814. He was one of the most noticeable members of the Reform Club, though he seemed always to avoid attracting attention; an enigmatical personage, about whom little was known, except that he was a polished man of the world.',
  },
  {
    id: 'the-picture-of-dorian-gray',
    title: 'The Picture of Dorian Gray',
    author: 'Oscar Wilde',
    narrator: 'Thomas Aldridge',
    year: 1890,
    duration: '8:15',
    genres: ['Classics', 'Gothic'],
    summary: 'A beautiful young man wishes that his portrait would age instead of him. The wish is granted, and Dorian Gray discovers exactly what a person can get away with.',
    cover: 225,
    coverStyle: 'gothic',
    chapters: numbered(20),
    added: 60,
    voice: 'Daniel',
    opening:
      'Chapter One. The studio was filled with the rich odour of roses, and when the light summer wind stirred amidst the trees of the garden, there came through the open door the heavy scent of the lilac, or the more delicate perfume of the pink-flowering thorn.',
  },
  {
    id: 'anne-of-green-gables',
    title: 'Anne of Green Gables',
    author: 'L. M. Montgomery',
    narrator: 'Eleanor Whitby',
    year: 1908,
    duration: '11:20',
    genres: ['Classics', 'Family'],
    series: { name: 'Anne of Green Gables', index: 1 },
    summary: 'Elderly siblings Matthew and Marilla Cuthbert send for an orphan boy to help on their Prince Edward Island farm. They get Anne Shirley instead — red-haired, talkative and utterly impossible to send back.',
    cover: 212,
    coverStyle: 'kids',
    chapters: numbered(38),
    added: 250,
    voice: 'Samantha',
    opening:
      'Chapter One. Mrs. Rachel Lynde Is Surprised. Mrs. Rachel Lynde lived just where the Avonlea main road dipped down into a little hollow, fringed with alders and ladies\' eardrops and traversed by a brook that had its source away back in the woods of the old Cuthbert place.',
  },
];

function chaptersFor(id: string, names: string[], total: number): AudiobookChapter[] {
  const r = rng(id);
  const weights = names.map(() => 0.7 + r() * 0.6);
  const sum = weights.reduce((a, b) => a + b, 0);
  let t = 0;
  return names.map((title, i) => {
    const duration = Math.round((weights[i] / sum) * total);
    const c = { title, start: t, duration };
    t += duration;
    return c;
  });
}

export const audiobooks: Audiobook[] = audiobookDefs.map((d) => {
  const [h, m] = d.duration.split(':').map(Number);
  const duration = h * 3600 + m * 60;
  const cover = art.cover(d.id);
  return {
    id: d.id,
    kind: 'audiobook',
    title: d.title,
    sortTitle: d.title.replace(/^(The|A|An) /, ''),
    author: d.author,
    narrator: d.narrator,
    year: d.year,
    published: daysAgo(900 + d.id.length * 20).slice(0, 10),
    duration,
    genres: d.genres,
    summary: d.summary,
    series: d.series,
    chapters: chaptersFor(d.id, d.chapters, duration),
    publisher: 'Portico Readers (fixture edition of a public-domain text)',
    libraryId: 'lib-audiobooks',
    art: { square: cover },
    palette: paletteFor(cover),
    addedAt: daysAgo(d.added, 10),
    source: { kind: 'public-domain', credit: `${d.title} by ${d.author} is in the public domain. Narration is synthesized for the fixture.`, license: 'Public domain' },
    streamId: 'spoken',
  };
});
