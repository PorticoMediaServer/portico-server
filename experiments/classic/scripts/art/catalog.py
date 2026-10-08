#!/usr/bin/env python3
"""
Builds Portico Classic's fixture catalogue: every library title, person, album,
book and photo, with deterministic metadata. Writes:

  src/data/fixtures/catalogData.json   (read by the fixture services)
  .art-cache/plan.json             (read by fetch.py and render.py)

Real titles are public-domain films and Blender Foundation open movies (CC-BY);
everything else is invented. Run: python3 scripts/art/catalog.py
"""
import json, os, random, re

ROOT = os.path.abspath(os.path.join(os.path.dirname(__file__), '..', '..'))
rng = random.Random(20260926)

def slug(s):
    return re.sub(r'[^a-z0-9]+', '-', s.lower()).strip('-')[:48]

# ── People ─────────────────────────────────────────────────────────────────
people = {}
def person(name, wiki=None, born=None, died=None, bio=None, known=None):
    pid = 'p-' + slug(name)
    if pid not in people:
        people[pid] = {'id': pid, 'name': name, 'wiki': wiki, 'born': born, 'died': died, 'bio': bio, 'knownFor': known, 'real': bool(wiki)}
    return pid

# ── Real films (public domain in the US, and Blender open movies) ──────────
# (title, year, minutes, rating, genres, directors, cast[(name, character)], synopsis, wiki article, commons category)
REAL = [
 ("Metropolis", 1927, 153, "NR", ["Science Fiction", "Drama"], ["Fritz Lang"], [("Brigitte Helm", "Maria"), ("Alfred Abel", "Joh Fredersen"), ("Gustav Fröhlich", "Freder"), ("Rudolf Klein-Rogge", "Rotwang")], "In a towering city split between thinkers above and workers below, the son of the city's master falls for a prophet of the underground, and an inventor builds a machine in her image.", "Metropolis (1927 film)", "Metropolis (1927 film)"),
 ("Nosferatu", 1922, 94, "NR", ["Horror"], ["F. W. Murnau"], [("Max Schreck", "Count Orlok"), ("Gustav von Wangenheim", "Thomas Hutter"), ("Greta Schröder", "Ellen Hutter")], "A young estate agent travels to the Carpathians to sell a house to a reclusive count, and brings something terrible home with him.", "Nosferatu", "Nosferatu (1922 film)"),
 ("The General", 1926, 78, "NR", ["Comedy", "Adventure"], ["Buster Keaton", "Clyde Bruckman"], [("Buster Keaton", "Johnnie Gray"), ("Marion Mack", "Annabelle Lee")], "A railroad engineer chases the locomotive he loves, and the woman aboard it, deep behind enemy lines.", "The General (1926 film)", "The General (1926 film)"),
 ("Sherlock Jr.", 1924, 45, "NR", ["Comedy"], ["Buster Keaton"], [("Buster Keaton", "Projectionist"), ("Kathryn McGuire", "The Girl")], "A projectionist who dreams of being a detective falls asleep at work and walks straight into the film on the screen.", "Sherlock Jr.", "Sherlock Jr."),
 ("Safety Last!", 1923, 70, "NR", ["Comedy"], ["Fred C. Newmeyer", "Sam Taylor"], [("Harold Lloyd", "The Boy"), ("Mildred Davis", "The Girl")], "A shop clerk trying to impress his sweetheart ends up climbing the outside of a department store, floor by perilous floor.", "Safety Last!", "Safety Last!"),
 ("The Kid", 1921, 68, "NR", ["Comedy", "Drama"], ["Charlie Chaplin"], [("Charlie Chaplin", "The Tramp"), ("Jackie Coogan", "The Kid"), ("Edna Purviance", "The Woman")], "The Tramp finds an abandoned baby and raises him as his own, until the authorities come calling.", "The Kid (1921 film)", "The Kid (1921 film)"),
 ("The Gold Rush", 1925, 95, "NR", ["Comedy", "Adventure"], ["Charlie Chaplin"], [("Charlie Chaplin", "The Lone Prospector"), ("Georgia Hale", "Georgia"), ("Mack Swain", "Big Jim McKay")], "A hopeful prospector braves the Klondike, a blizzard-bound cabin and a very hungry partner in search of gold and love.", "The Gold Rush", "The Gold Rush"),
 ("His Girl Friday", 1940, 92, "NR", ["Comedy", "Romance"], ["Howard Hawks"], [("Cary Grant", "Walter Burns"), ("Rosalind Russell", "Hildy Johnson"), ("Ralph Bellamy", "Bruce Baldwin")], "A fast-talking editor learns his star reporter, and ex-wife, is about to remarry, and dangles one last scoop to keep her.", "His Girl Friday", "His Girl Friday"),
 ("Charade", 1963, 113, "NR", ["Thriller", "Romance", "Comedy"], ["Stanley Donen"], [("Cary Grant", "Peter Joshua"), ("Audrey Hepburn", "Regina Lampert"), ("Walter Matthau", "Hamilton Bartholomew"), ("James Coburn", "Tex Panthollow")], "After her husband is murdered, a young widow in Paris is pursued by men who want the fortune he stole, and helped by a stranger she can't quite trust.", "Charade (1963 film)", "Charade (1963 film)"),
 ("Night of the Living Dead", 1968, 96, "NR", ["Horror"], ["George A. Romero"], [("Duane Jones", "Ben"), ("Judith O'Dea", "Barbra")], "Strangers barricade themselves in a Pennsylvania farmhouse as the recently dead rise and surround them.", "Night of the Living Dead", "Night of the Living Dead"),
 ("The Cabinet of Dr. Caligari", 1920, 76, "NR", ["Horror", "Mystery"], ["Robert Wiene"], [("Werner Krauss", "Dr. Caligari"), ("Conrad Veidt", "Cesare"), ("Lil Dagover", "Jane")], "A fairground hypnotist and his sleepwalking servant arrive in a crooked little town, and the killings begin.", "The Cabinet of Dr. Caligari", "The Cabinet of Dr. Caligari"),
 ("The Phantom of the Opera", 1925, 93, "NR", ["Horror", "Romance"], ["Rupert Julian"], [("Lon Chaney", "The Phantom"), ("Mary Philbin", "Christine Daaé"), ("Norman Kerry", "Raoul de Chagny")], "Beneath the Paris Opera a masked genius tutors a young soprano, and will stop at nothing to make her a star.", "The Phantom of the Opera (1925 film)", "The Phantom of the Opera (1925 film)"),
 ("Carnival of Souls", 1962, 78, "NR", ["Horror", "Mystery"], ["Herk Harvey"], [("Candace Hilligoss", "Mary Henry")], "After surviving a car accident, a church organist is drawn toward an abandoned lakeside pavilion by a pale, persistent figure.", "Carnival of Souls", "Carnival of Souls"),
 ("The Little Shop of Horrors", 1960, 72, "NR", ["Comedy", "Horror"], ["Roger Corman"], [("Jonathan Haze", "Seymour Krelboyne"), ("Jackie Joseph", "Audrey Fulquard"), ("Jack Nicholson", "Wilbur Force")], "A hapless florist's assistant breeds a plant with a taste for people, and business has never been better.", "The Little Shop of Horrors", "The Little Shop of Horrors"),
 ("Plan 9 from Outer Space", 1957, 79, "NR", ["Science Fiction", "Horror"], ["Ed Wood"], [("Bela Lugosi", "Ghoul Man"), ("Tor Johnson", "Inspector Clay")], "Aliens resurrect the dead of a California cemetery to stop humanity from building a doomsday weapon.", "Plan 9 from Outer Space", "Plan 9 from Outer Space"),
 ("Detour", 1945, 68, "NR", ["Crime", "Thriller"], ["Edgar G. Ulmer"], [("Tom Neal", "Al Roberts"), ("Ann Savage", "Vera")], "A nightclub pianist hitchhiking to Los Angeles makes one bad decision on the road, and then a worse one.", "Detour (1945 film)", "Detour (1945 film)"),
 ("D.O.A.", 1950, 83, "NR", ["Crime", "Thriller"], ["Rudolph Maté"], [("Edmond O'Brien", "Frank Bigelow"), ("Pamela Britton", "Paula Gibson")], "A man learns he has been poisoned and has days to live, just long enough to find out who killed him.", "D.O.A. (1950 film)", "D.O.A. (1950 film)"),
 ("The Stranger", 1946, 95, "NR", ["Thriller", "Drama"], ["Orson Welles"], [("Edward G. Robinson", "Mr. Wilson"), ("Loretta Young", "Mary Longstreet"), ("Orson Welles", "Professor Charles Rankin")], "A war-crimes investigator tracks a fugitive to a quiet Connecticut town, where he has married into a respected family.", "The Stranger (1946 film)", "The Stranger (1946 film)"),
 ("Scarlet Street", 1945, 102, "NR", ["Crime", "Drama"], ["Fritz Lang"], [("Edward G. Robinson", "Christopher Cross"), ("Joan Bennett", "Kitty March"), ("Dan Duryea", "Johnny Prince")], "A lonely cashier and Sunday painter is drawn into a con by a woman and her grifter boyfriend.", "Scarlet Street", "Scarlet Street"),
 ("Royal Wedding", 1951, 93, "NR", ["Musical", "Romance"], ["Stanley Donen"], [("Fred Astaire", "Tom Bowen"), ("Jane Powell", "Ellen Bowen")], "A brother-and-sister dance act takes their show to London during the royal wedding, and both fall in love.", "Royal Wedding", "Royal Wedding"),
 ("Meet John Doe", 1941, 122, "NR", ["Drama", "Comedy"], ["Frank Capra"], [("Gary Cooper", "Long John Willoughby"), ("Barbara Stanwyck", "Ann Mitchell")], "A columnist invents a man who will protest society's ills by jumping from a roof, then hires a drifter to play him.", "Meet John Doe", "Meet John Doe"),
 ("My Man Godfrey", 1936, 94, "NR", ["Comedy", "Romance"], ["Gregory La Cava"], [("William Powell", "Godfrey"), ("Carole Lombard", "Irene Bullock")], "A scatterbrained heiress hires a man from the city dump as her family's butler, and he quietly sets them all straight.", "My Man Godfrey", "My Man Godfrey"),
 ("The Last Man on Earth", 1964, 86, "NR", ["Horror", "Science Fiction"], ["Ubaldo Ragona", "Sidney Salkow"], [("Vincent Price", "Dr. Robert Morgan")], "The lone survivor of a plague spends his days hunting the infected and his nights besieged by them.", "The Last Man on Earth (1964 film)", "The Last Man on Earth (1964 film)"),
 ("House on Haunted Hill", 1959, 75, "NR", ["Horror"], ["William Castle"], [("Vincent Price", "Frederick Loren")], "An eccentric millionaire offers five guests ten thousand dollars each to survive a night in a haunted house.", "House on Haunted Hill", "House on Haunted Hill"),
 ("McLintock!", 1963, 127, "NR", ["Western", "Comedy"], ["Andrew V. McLaglen"], [("John Wayne", "George Washington McLintock"), ("Maureen O'Hara", "Katherine McLintock")], "A cattle baron's estranged wife returns to town, and the fireworks start before she's unpacked.", "McLintock!", "McLintock!"),
 ("Suddenly", 1954, 77, "NR", ["Crime", "Thriller"], ["Lewis Allen"], [("Frank Sinatra", "John Baron"), ("Sterling Hayden", "Sheriff Tod Shaw")], "Hired gunmen take over a hillside house in a sleepy town to ambush the President's train.", "Suddenly (1954 film)", "Suddenly (1954 film)"),
 ("Beat the Devil", 1953, 89, "NR", ["Comedy", "Adventure"], ["John Huston"], [("Humphrey Bogart", "Billy Dannreuther"), ("Jennifer Jones", "Gwendolen Chelm"), ("Gina Lollobrigida", "Maria Dannreuther")], "A gang of mismatched crooks waiting in an Italian port for a ship to Africa plots to grab uranium-rich land.", "Beat the Devil (film)", "Beat the Devil (film)"),
 ("Nothing Sacred", 1937, 77, "NR", ["Comedy"], ["William A. Wellman"], [("Carole Lombard", "Hazel Flagg"), ("Fredric March", "Wally Cook")], "A small-town woman told she is dying becomes a newspaper sensation in New York, and the diagnosis turns out to be wrong.", "Nothing Sacred (film)", "Nothing Sacred (film)"),
 ("A Star Is Born", 1937, 111, "NR", ["Drama", "Romance"], ["William A. Wellman"], [("Janet Gaynor", "Esther Blodgett"), ("Fredric March", "Norman Maine")], "A farm girl becomes a Hollywood star as the fading actor who discovered her slides toward ruin.", "A Star Is Born (1937 film)", "A Star Is Born (1937 film)"),
 ("Penny Serenade", 1941, 119, "NR", ["Drama", "Romance"], ["George Stevens"], [("Irene Dunne", "Julie Gardiner Adams"), ("Cary Grant", "Roger Adams")], "Playing old records, a woman remembers her marriage, from its first song to its hardest days.", "Penny Serenade", "Penny Serenade"),
 ("The Man with the Golden Arm", 1955, 119, "NR", ["Drama"], ["Otto Preminger"], [("Frank Sinatra", "Frankie Machine"), ("Kim Novak", "Molly"), ("Eleanor Parker", "Zosch Machine")], "A card dealer home from prison tries to become a jazz drummer and stay clean on a street that won't let him.", "The Man with the Golden Arm", "The Man with the Golden Arm"),
 ("Angel and the Badman", 1947, 100, "NR", ["Western", "Romance"], ["James Edward Grant"], [("John Wayne", "Quirt Evans"), ("Gail Russell", "Penelope Worth")], "A wounded gunfighter is nursed back to health by a Quaker family, and their daughter makes him question his ways.", "Angel and the Badman", "Angel and the Badman"),
 ("Dementia 13", 1963, 75, "NR", ["Horror", "Thriller"], ["Francis Ford Coppola"], [("William Campbell", "Richard Haloran"), ("Luana Anders", "Louise Haloran")], "A scheming widow hides her husband's death from his mother at their Irish castle, where an axe murderer waits.", "Dementia 13", "Dementia 13"),
 ("The Terror", 1963, 81, "NR", ["Horror"], ["Roger Corman"], [("Boris Karloff", "Baron Victor Frederick Von Leppe"), ("Jack Nicholson", "Lt. André Duvalier")], "A lost French officer follows a mysterious woman to a baron's castle on the Baltic coast.", "The Terror (1963 film)", "The Terror (1963 film)"),
 ("The Hitch-Hiker", 1953, 71, "NR", ["Crime", "Thriller"], ["Ida Lupino"], [("Edmond O'Brien", "Roy Collins"), ("Frank Lovejoy", "Gilbert Bowen")], "Two friends on a fishing trip pick up a hitchhiker who turns out to be a killer on the run.", "The Hitch-Hiker (1953 film)", "The Hitch-Hiker (1953 film)"),
 ("Kansas City Confidential", 1952, 99, "NR", ["Crime"], ["Phil Karlson"], [("John Payne", "Joe Rolfe"), ("Coleen Gray", "Helen Foster")], "Framed for an armored-car robbery, a delivery driver tracks the masked gang to a Mexican resort.", "Kansas City Confidential", "Kansas City Confidential"),
 ("Gulliver's Travels", 1939, 76, "G", ["Animation", "Family", "Adventure"], ["Dave Fleischer"], [], "A shipwrecked sailor washes up in Lilliput, where two tiny kingdoms are about to go to war over a wedding song.", "Gulliver's Travels (1939 film)", "Gulliver's Travels (1939 film)"),
 ("A Trip to the Moon", 1902, 13, "G", ["Science Fiction", "Fantasy"], ["Georges Méliès"], [("Georges Méliès", "Professor Barbenfouillis")], "Astronomers fire themselves at the Moon in a capsule and meet its inhabitants in the first great science-fiction film.", "A Trip to the Moon", "A Trip to the Moon"),
 ("The Great Train Robbery", 1903, 12, "NR", ["Western"], ["Edwin S. Porter"], [], "Bandits hold up a steam train and are chased down by a posse in one of the first narrative westerns.", "The Great Train Robbery (1903 film)", "The Great Train Robbery (1903 film)"),
 ("Battleship Potemkin", 1925, 75, "NR", ["Drama", "History"], ["Sergei Eisenstein"], [], "The crew of a battleship rebels against its officers, and the port city of Odessa rises to meet them.", "Battleship Potemkin", "Battleship Potemkin"),
 ("Nanook of the North", 1922, 79, "G", ["Documentary"], ["Robert J. Flaherty"], [], "A year in the life of an Inuk hunter and his family on the shores of Hudson Bay.", "Nanook of the North", "Nanook of the North"),
 ("The Lost World", 1925, 106, "NR", ["Adventure", "Science Fiction"], ["Harry O. Hoyt"], [("Wallace Beery", "Professor Challenger"), ("Bessie Love", "Paula White")], "An expedition to a remote South American plateau finds dinosaurs still alive, and brings one back to London.", "The Lost World (1925 film)", "The Lost World (1925 film)"),
 ("The Hunchback of Notre Dame", 1923, 117, "NR", ["Drama", "Romance"], ["Wallace Worsley"], [("Lon Chaney", "Quasimodo"), ("Patsy Ruth Miller", "Esmeralda")], "The bell-ringer of Notre Dame gives everything to protect a street dancer condemned by the powerful.", "The Hunchback of Notre Dame (1923 film)", "The Hunchback of Notre Dame (1923 film)"),
 ("Steamboat Bill, Jr.", 1928, 71, "NR", ["Comedy"], ["Charles Reisner"], [("Buster Keaton", "William Canfield Jr."), ("Ernest Torrence", "William Canfield Sr.")], "A college boy returns to help his riverboat-captain father, just in time for the storm of the century.", "Steamboat Bill, Jr.", "Steamboat Bill, Jr."),
 ("The Navigator", 1924, 59, "NR", ["Comedy"], ["Buster Keaton", "Donald Crisp"], [("Buster Keaton", "Rollo Treadway"), ("Kathryn McGuire", "Betsy O'Brien")], "Two pampered heirs are adrift alone on an empty ocean liner and must learn to fend for themselves.", "The Navigator (1924 film)", "The Navigator (1924 film)"),
 ("The Freshman", 1925, 76, "NR", ["Comedy"], ["Fred C. Newmeyer", "Sam Taylor"], [("Harold Lloyd", "Harold Lamb"), ("Jobyna Ralston", "Peggy")], "An eager college freshman will do anything to be popular, including joining the football team.", "The Freshman (1925 film)", "The Freshman (1925 film)"),
 ("The Mark of Zorro", 1920, 90, "NR", ["Adventure", "Romance"], ["Fred Niblo"], [("Douglas Fairbanks", "Don Diego Vega / Zorro"), ("Marguerite De La Motte", "Lolita Pulido")], "A foppish nobleman in old California is secretly the masked swordsman who defends the people.", "The Mark of Zorro (1920 film)", "The Mark of Zorro (1920 film)"),
 ("The Thief of Bagdad", 1924, 155, "NR", ["Adventure", "Fantasy"], ["Raoul Walsh"], [("Douglas Fairbanks", "Ahmed, the Thief")], "A thief who falls for a caliph's daughter sets off on a quest for a magic chest to win her hand.", "The Thief of Bagdad (1924 film)", "The Thief of Bagdad (1924 film)"),
 ("Robin Hood", 1922, 143, "NR", ["Adventure"], ["Allan Dwan"], [("Douglas Fairbanks", "Robin Hood"), ("Enid Bennett", "Lady Marian")], "The Earl of Huntingdon becomes an outlaw of Sherwood Forest to defend England from the usurping Prince John.", "Robin Hood (1922 film)", "Robin Hood (1922 film)"),
 ("The Sheik", 1921, 80, "NR", ["Romance", "Adventure"], ["George Melford"], [("Rudolph Valentino", "Sheik Ahmed Ben Hassan"), ("Agnes Ayres", "Lady Diana Mayo")], "A headstrong Englishwoman travelling in the Sahara is taken captive by a desert prince.", "The Sheik (film)", "The Sheik (film)"),
 ("Sunrise: A Song of Two Humans", 1927, 94, "NR", ["Drama", "Romance"], ["F. W. Murnau"], [("George O'Brien", "The Man"), ("Janet Gaynor", "The Wife")], "A farmer tempted to drown his wife finds, over one day in the city, that he loves her after all.", "Sunrise: A Song of Two Humans", "Sunrise: A Song of Two Humans"),
 ("Wings", 1927, 144, "NR", ["War", "Drama"], ["William A. Wellman"], [("Clara Bow", "Mary Preston"), ("Charles 'Buddy' Rogers", "Jack Powell"), ("Richard Arlen", "David Armstrong")], "Two small-town rivals become fighter pilots and friends over the Western Front.", "Wings (1927 film)", "Wings (1927 film)"),
 ("The Passion of Joan of Arc", 1928, 82, "NR", ["Drama", "History"], ["Carl Theodor Dreyer"], [("Renée Jeanne Falconetti", "Jeanne d'Arc")], "The trial of Joan of Arc, told almost entirely in close-up, from interrogation to the stake.", "The Passion of Joan of Arc", "The Passion of Joan of Arc"),
 ("Faust", 1926, 116, "NR", ["Fantasy", "Horror"], ["F. W. Murnau"], [("Gösta Ekman", "Faust"), ("Emil Jannings", "Mephisto")], "An aging alchemist bargains his soul for youth, and the devil makes sure he pays.", "Faust (1926 film)", "Faust (1926 film)"),
 ("The Man Who Laughs", 1928, 110, "NR", ["Drama", "Romance"], ["Paul Leni"], [("Conrad Veidt", "Gwynplaine"), ("Mary Philbin", "Dea")], "A travelling performer whose face was carved into a permanent grin falls for a blind girl who can't see it.", "The Man Who Laughs (1928 film)", "The Man Who Laughs (1928 film)"),
 ("The Crowd", 1928, 98, "NR", ["Drama"], ["King Vidor"], [("Eleanor Boardman", "Mary Sims"), ("James Murray", "John Sims")], "An ordinary man arrives in New York sure he will be somebody, and the city swallows him whole.", "The Crowd (1928 film)", "The Crowd (1928 film)"),
 ("The Wind", 1928, 95, "NR", ["Drama", "Western"], ["Victor Sjöström"], [("Lillian Gish", "Letty Mason")], "A young woman from Virginia moves to the Texas plains, where the endless wind starts to break her.", "The Wind (1928 film)", "The Wind (1928 film)"),
 ("It", 1927, 72, "NR", ["Comedy", "Romance"], ["Clarence G. Badger"], [("Clara Bow", "Betty Lou Spence")], "A shop girl with that certain something sets her sights on the handsome owner of the store.", "It (1927 film)", "It (1927 film)"),
 ("The Lodger", 1927, 90, "NR", ["Thriller", "Mystery"], ["Alfred Hitchcock"], [("Ivor Novello", "The Lodger"), ("June Tripp", "Daisy Bunting")], "As a killer stalks foggy London, a family begins to suspect the strange young man renting their upstairs room.", "The Lodger: A Story of the London Fog", "The Lodger (1927 film)"),
 ("Africa Screams", 1949, 79, "NR", ["Comedy", "Adventure"], ["Charles Barton"], [("Bud Abbott", "Buzz Johnson"), ("Lou Costello", "Stanley Livington")], "Two department-store clerks bluff their way onto an African safari after a secret diamond mine.", "Africa Screams", "Africa Screams"),
 ("The Brain That Wouldn't Die", 1962, 82, "NR", ["Science Fiction", "Horror"], ["Joseph Green"], [("Jason Evers", "Dr. Bill Cortner"), ("Virginia Leith", "Jan Compton")], "A surgeon keeps his fiancée's head alive after a car crash and goes looking for a new body.", "The Brain That Wouldn't Die", "The Brain That Wouldn't Die"),
 ("Topper Returns", 1941, 88, "NR", ["Comedy", "Mystery"], ["Roy Del Ruth"], [("Joan Blondell", "Gail Richards"), ("Roland Young", "Cosmo Topper")], "A murdered woman's ghost recruits a nervous banker to find her killer before he strikes again.", "Topper Returns", "Topper Returns"),
 ("Father's Little Dividend", 1951, 82, "NR", ["Comedy", "Family"], ["Vincente Minnelli"], [("Spencer Tracy", "Stanley T. Banks"), ("Joan Bennett", "Ellie Banks"), ("Elizabeth Taylor", "Kay Dunstan")], "A father just recovering from his daughter's wedding learns he is about to become a grandfather.", "Father's Little Dividend", "Father's Little Dividend"),
 ("Road to Bali", 1952, 91, "NR", ["Comedy", "Musical"], ["Hal Walker"], [("Bob Hope", "Harold Gridley"), ("Bing Crosby", "George Cochran"), ("Dorothy Lamour", "Princess Lalah")], "Two song-and-dance men on the run take jobs as divers in the South Seas and compete for a princess.", "Road to Bali", "Road to Bali"),
 ("Jack and the Beanstalk", 1952, 78, "G", ["Comedy", "Family", "Fantasy"], ["Jean Yarbrough"], [("Bud Abbott", "Dinkelpuss"), ("Lou Costello", "Jack")], "A babysitter reading a fairy tale dreams himself into it, beans, beanstalk and giant included.", "Jack and the Beanstalk (1952 film)", "Jack and the Beanstalk (1952 film)"),
 ("Of Human Bondage", 1934, 83, "NR", ["Drama", "Romance"], ["John Cromwell"], [("Leslie Howard", "Philip Carey"), ("Bette Davis", "Mildred Rogers")], "A sensitive medical student becomes obsessed with a cruel waitress who uses him again and again.", "Of Human Bondage (1934 film)", "Of Human Bondage (1934 film)"),
 ("Intolerance", 1916, 163, "NR", ["Drama", "History"], ["D. W. Griffith"], [("Lillian Gish", "The Woman Who Rocks the Cradle")], "Four stories across twenty-five centuries show how intolerance has shaped history.", "Intolerance (film)", "Intolerance (film)"),
]

BLENDER = [
 ("Big Buck Bunny", 2008, 10, "G", ["Animation", "Comedy", "Family"], ["Sacha Goedegebure"], "A gentle giant rabbit takes revenge on three bullying forest rodents in a rollicking chase through the meadow.", "Big Buck Bunny", "Big Buck Bunny"),
 ("Elephants Dream", 2006, 11, "PG", ["Animation", "Science Fiction"], ["Bassam Kurdali"], "Two men explore a vast, shifting machine world, one convinced it is full of wonders, the other sure it is dangerous.", "Elephants Dream", "Elephants Dream"),
 ("Sintel", 2010, 15, "PG-13", ["Animation", "Fantasy", "Adventure"], ["Colin Levy"], "A lone young woman crosses snow and desert in search of the baby dragon she once rescued.", "Sintel", "Sintel"),
 ("Tears of Steel", 2012, 12, "PG-13", ["Science Fiction", "Action"], ["Ian Hubert"], "In a future Amsterdam, a group of scientists tries to rescue the world from robots by replaying a painful breakup.", "Tears of Steel", "Tears of Steel"),
 ("Cosmos Laundromat", 2015, 12, "PG", ["Animation", "Fantasy", "Comedy"], ["Mathieu Auvray"], "A suicidal sheep on a desolate island meets a salesman who offers him the gift of a lifetime, many lifetimes, in fact.", "Cosmos Laundromat", "Cosmos Laundromat"),
 ("Spring", 2019, 8, "G", ["Animation", "Fantasy", "Family"], ["Andy Goralczyk"], "A shepherd girl and her dog face ancient spirits to bring the cycle of life back to their mountain.", "Spring (2019 film)", "Spring (2019 film)"),
 ("Charge", 2022, 4, "PG-13", ["Animation", "Action"], [], "In a harsh, energy-starved future, an old man guards the last working battery against a determined thief.", "Charge (2022 film)", "Charge (Blender open movie)"),
 ("Sprite Fright", 2021, 10, "PG-13", ["Animation", "Comedy", "Horror"], ["Matthew Luhn"], "Rowdy teenagers on a woodland trip discover the tiny mushroom folk they have been trampling are not so friendly.", "Sprite Fright", "Sprite Fright"),
 ("Agent 327: Operation Barbershop", 2017, 4, "PG", ["Animation", "Action", "Comedy"], ["Colin Levy", "Hjalti Hjálmarsson"], "A secret agent goes undercover in a barbershop that hides a much less friendly secret.", "Agent 327: Operation Barbershop", "Agent 327: Operation Barbershop"),
 ("Coffee Run", 2020, 3, "G", ["Animation"], ["Hjalti Hjálmarsson"], "Fuelled by caffeine, a young woman races through memories of a relationship she is trying to leave behind.", "Coffee Run (film)", "Coffee Run"),
]

# ── Invented films ─────────────────────────────────────────────────────────
INVENTED_MOVIES = """
Saltwater Hymn|Low Tide at Kessock|The Quiet Year|Northbound|Glasshouse|Meridian|Paper Lanterns|Undertow|Stillwater Road|The Cartographer's Daughter|Ember & Ash|Harbor Lights|Nine Winters|The Long Field|Signal Fire|Blue Hour|A House on Lark Street|The Orchard Keepers|Copper Sky|Fathom|Wolf Moon Rising|The Lighthouse at Vane Point|Driftwood|Hollow Crown Hotel|Parallel Lines|The Weight of Snow|Starling|Half Light|Salt & Iron|The Last Ferry|Silver Creek|Nightjar|The Glass Orchard|Kingfisher|Tidewater|Open Range|Evening Standard|The Understudy|Small Hours|Iron Bloom|Riverbend|The Honest Thief|Dust Devils|Cold Harbor|Lantern Festival|Winter Garden|A Room of Echoes|The Pale Horizon|Crossing Kessler|The Silent Partner|Sunday Drivers|Anchor Point|Brass Monkey|The Violet Hour|Quarry|Ghost Orchid|Frontier Blues|Palisade|The Paper Crane|Waiting for Rain|Foxglove|The Stone Choir|Black Ridge|Neon Saints|Holloway|Coastline|The Fifth Season|Deep Water Blues|Arcadia Falls|Echo Canyon|The Keeper's Log|Marigold|Second Wind|Lowlands|Red Rock Crossing|The Night Mail|Wildflower|Solace|The Burning Coast|Firebreak|Moth & Flame|The Archivist|City of Glass|Lake Effect|Dead Reckoning|The Gilded Cage|Harvest Moon|After the Flood|Blackwater|The Tin Soldier|Distant Thunder|Hinterland|Long Division|The Painted Veil of Oren|Night Shift|Canary Row|Cloudline|The Map Room|Snowbound|Vesper|Thin Ice|The Last Good Summer|Wanderlust|Radio Silence|Ironwood|Tumbleweed|The Clockmaker|Borderlands|Blue Mesa|Kindling|Spindrift|The Wake|Foreign Soil|North Star Motel|Sandcastle|Coldwater Creek|The Night Garden|Afterglow|Heavy Weather|Crosswind|The Salt Road|Twelve Bells|Lamplighter|Everglade|The Winter Palace|Brightwater|Salvage|Hearthstone|Riptide|The Sparrow's Song|Cinder|White Noise Radio|Aurora Line|Parlor Games|The Gray Lady|Low Country|Heartwood|The Understory|Longshore|Wanderers|Tall Grass|Hush|The Iron Horse|Seawall|Sweetwater|Mountain Time|Fever Dream|The Rookery|Night Train to Varna|Lost Signal|Monument|The Almanac|Duskfall|Southern Cross|Pilgrim|Farewell, Oriole|The Ferryman|Magnolia Street|Wild Card|Last Light|The Collector|Vantage|Emberly|Moonraker Lane|Gravity Well|Fault Lines|The Stowaway|Honeycomb|Barnstormers|Salt Flats|Marrow|The Swimmer's Year|Headwaters|Glimmer|The Forgery|Overcast|Thunder Road Diner|Harrow|Firefly Summer|The Night Watchman|Aftershock|Bellwether|Kestrel|Downriver|Terminal Velocity|Outpost Nine|The Understudy's Ghost|Pale Rider Pass|Neon Coast|Wreckers|Carousel|The Tollkeeper|Longwave|Gold Coast Local|Bitter Harvest|Fog Line|The Descent at Kiruna|Parallax|Station Eleven West|The Last Orbit|Zero Meridian|Halcyon|Deep Field|Orbital|Red Shift|The Europa Signal|Afterimage|Voidrunner|Circuit Breaker|Lumen|Paradox Engine|Synthetic Dawn|Cold Fusion|The Hollow Moon|Starfall|Nightfall Protocol|Quiet Planet|Ion Storm|The Gardener of Mars|Ascension Day|Blindspot|Kill Switch|Double Blind|The Fixer|Crossfire|Bullet Train to Osaka|Point Blank Range|Shadow Protocol|Hard Target|The Heist at Hollin Bank|Getaway Driver|Burn Notice Road|Iron Fist Alley|Red Zone|Countdown|The Extraction|Full Throttle|Deadline|The Negotiator|Collateral Line|Snake Eyes|Sudden Impact Bay|The Watchers|Hollow Pines|The Night Visitor|Crawlspace|Whisper Room|The Tenant Upstairs|Bone Orchard|Salt Witch|The Dollmaker|Candle Cove Road|Pale Hands|Dead Air|Harrowgate|The Offering|Wicker Lane|Shiver|Blackthorn|The Hollowing|Grin|Nightwalker|Mothman of Pell|Beneath the Floorboards|Laugh Track|Just Friends|Wedding Season|The Plus One|Meet the Parents Again|Second Date|Love, Actually Maybe|Road Trip Rules|The Best Man's Toast|Roommates|Office Hours|Summer Camp Forever|Bad Neighbors|The Big Weekend|Dinner for Twelve|Brunch Club|Holiday Rush|Game Night|Weekend at Aunt Mae's|Karaoke Kings
""".strip().split('|')

GENRE_POOL = {
 'drama': ["Drama"], 'thriller': ["Thriller", "Crime"], 'scifi': ["Science Fiction"], 'action': ["Action", "Thriller"], 'horror': ["Horror"], 'comedy': ["Comedy"], 'romance': ["Romance", "Drama"], 'western': ["Western"], 'mystery': ["Mystery", "Thriller"], 'adventure': ["Adventure"], 'war': ["War", "Drama"], 'family': ["Family", "Animation"], 'music': ["Music", "Drama"], 'documentary': ["Documentary"],
}
SCIFI = set("Parallax|Station Eleven West|The Last Orbit|Zero Meridian|Halcyon|Deep Field|Orbital|Red Shift|The Europa Signal|Afterimage|Voidrunner|Circuit Breaker|Lumen|Paradox Engine|Synthetic Dawn|Cold Fusion|The Hollow Moon|Starfall|Nightfall Protocol|Quiet Planet|Ion Storm|The Gardener of Mars|Ascension Day|Gravity Well|Outpost Nine|Terminal Velocity|Aurora Line|Lost Signal".split('|'))
ACTION = set("Blindspot|Kill Switch|Double Blind|The Fixer|Crossfire|Bullet Train to Osaka|Point Blank Range|Shadow Protocol|Hard Target|The Heist at Hollin Bank|Getaway Driver|Burn Notice Road|Iron Fist Alley|Red Zone|Countdown|The Extraction|Full Throttle|Deadline|The Negotiator|Collateral Line|Snake Eyes|Sudden Impact Bay|Neon Saints|Wild Card|Salvage|Firebreak".split('|'))
HORROR = set("The Watchers|Hollow Pines|The Night Visitor|Crawlspace|Whisper Room|The Tenant Upstairs|Bone Orchard|Salt Witch|The Dollmaker|Candle Cove Road|Pale Hands|Dead Air|Harrowgate|The Offering|Wicker Lane|Shiver|Blackthorn|The Hollowing|Grin|Nightwalker|Mothman of Pell|Beneath the Floorboards|Ghost Orchid|Nightjar|Moth & Flame|The Rookery|Marrow|Harrow".split('|'))
COMEDY = set("Laugh Track|Just Friends|Wedding Season|The Plus One|Meet the Parents Again|Second Date|Love, Actually Maybe|Road Trip Rules|The Best Man's Toast|Roommates|Office Hours|Summer Camp Forever|Bad Neighbors|The Big Weekend|Dinner for Twelve|Brunch Club|Holiday Rush|Game Night|Weekend at Aunt Mae's|Karaoke Kings|Sunday Drivers|Brass Monkey|Parlor Games|Barnstormers|Thunder Road Diner|Carousel".split('|'))
WESTERN = set("Open Range|Dust Devils|Frontier Blues|Red Rock Crossing|Tumbleweed|Borderlands|Blue Mesa|Pale Rider Pass|The Iron Horse|Salt Flats|Echo Canyon|Black Ridge".split('|'))
FAMILY = set("Paper Lanterns|The Paper Crane|Sandcastle|Firefly Summer|Honeycomb|Wildflower|Marigold|The Tin Soldier|Kingfisher|Starling|The Sparrow's Song|Lantern Festival|Glimmer|Snowbound|The Clockmaker|The Almanac".split('|'))

# Logline pieces (invented, original copy).
WHO = ["a retired lighthouse keeper", "an ambitious young lawyer", "an estranged sister", "a small-town sheriff", "a disgraced chef", "a widowed ferry captain", "a teenage radio operator", "a jazz pianist down on her luck", "an insurance investigator", "a rival cartographer", "a marine biologist", "a runaway heir", "a stubborn orchard farmer", "a night-shift nurse", "a travelling magician", "a retired astronaut", "a museum night guard", "a bush pilot", "a burned-out detective", "a shy piano tuner", "a former champion boxer", "a war photographer", "a village schoolteacher", "a retired watchmaker", "a gifted forger", "a mountain rescue volunteer", "a struggling bookseller", "a courier", "a young architect", "a childhood friend"]
WHAT = ["returns home after twenty years", "inherits a crumbling seaside hotel", "discovers a letter that was never sent", "is hired to find a missing painting", "takes one last job", "is stranded by a storm", "agrees to a bet they can't afford to lose", "uncovers a secret buried under the town", "must deliver a package across three borders", "starts hearing a voice on a dead frequency", "is mistaken for someone else", "tries to save the family farm", "goes looking for a brother who vanished", "wakes up with no memory of the night before", "enters a contest nobody has ever won", "is asked to keep a stranger's child safe for one week", "becomes the reluctant guardian of a stray dog", "finds a map folded inside an old coat", "sets out to walk the length of the coast"]
THEN = ["and the past refuses to stay buried", "only to find the town has plans of its own", "while an unlikely friendship slowly takes root", "and learns that some debts are never repaid", "as winter closes every road out", "and nothing goes according to plan", "with a relentless rival one step behind", "and discovers what they were really running from", "until a single phone call changes everything", "and has to decide who they can trust", "as the tide rises on a long, strange night", "and falls in love along the way", "before the summer ends"]
SCIFI_LINES = ["A deep-space salvage crew answers a distress call from a ship that disappeared forty years ago.", "When the sun fails to rise over a remote research station, the last engineer on shift has twelve hours to find out why.", "A cartographer mapping a newly opened wormhole finds her own handwriting on the charts.", "The first colony on Europa goes silent, and the rescue team discovers the ice is listening.", "An android caretaker on an abandoned orbital farm starts planting things nobody asked for.", "Humanity's first message from the stars turns out to be a recording of ourselves.", "A courier running cargo between moons is offered one job that could buy her freedom.", "In a city where memories can be sold, a detective investigates a theft of the one thing that can't be copied."]
ACTION_LINES = ["A getaway driver is double-crossed on her final job and has one night to get even.", "An ex-operative living quietly by the sea is pulled back in when his old handler turns up dead.", "A bank heist in a snowbound mountain town goes wrong when the vault holds something nobody expected.", "A bodyguard must get a key witness across the city before sunrise with every crew in town hunting them.", "Trapped on a speeding overnight train, a negotiator talks down hijackers one carriage at a time.", "A disgraced agent races to stop a stolen satellite key from reaching the highest bidder."]
HORROR_LINES = ["A family moves into a farmhouse where the previous owners left every door locked from the outside.", "Campers on a lakeside weekend realise the woods go quiet at exactly the same minute every night.", "A night-shift radio host starts receiving calls from listeners who died years ago.", "A restorer of antique dolls finds each one she repairs has moved by morning.", "Something beneath the floorboards of an old boarding house is counting the tenants.", "A village celebrates its harvest the same way it has for three hundred years, and this year a stranger is invited."]
COMEDY_LINES = ["Two best friends accidentally book the same wedding venue on the same day and refuse to back down.", "A buttoned-up accountant is forced to join her family's chaotic road trip to a reunion nobody wants to attend.", "Roommates who can't stand each other pretend to be a couple to win an apartment.", "A washed-up karaoke champion trains a hopeless office team for the city finals.", "A dinner party for twelve spirals out of control when the host's ex arrives as someone's plus-one.", "Three brothers inherit their aunt's lakeside cottage on the condition that they spend one summer there together."]
WESTERN_LINES = ["A drifter rides into a dying frontier town and agrees to stand between its people and the railroad men.", "A widowed rancher and her hired hand drive a herd across three hundred miles of drought country.", "An aging marshal escorts a prisoner to trial through territory where everyone wants the man dead.", "Two brothers on opposite sides of the law meet again at a river crossing neither can avoid."]
FAMILY_LINES = ["A girl and her grandfather build a kite big enough to carry a letter to the moon.", "A lost fox cub and a lonely lighthouse cat set out across the island to find the cub's family.", "The children of a sleepy village discover the town clock has been keeping time for something magical.", "A young inventor enters her homemade robot in the county fair, and it has ideas of its own."]

def logline(title):
    if title in SCIFI: return rng.choice(SCIFI_LINES)
    if title in ACTION: return rng.choice(ACTION_LINES)
    if title in HORROR: return rng.choice(HORROR_LINES)
    if title in COMEDY: return rng.choice(COMEDY_LINES)
    if title in WESTERN: return rng.choice(WESTERN_LINES)
    if title in FAMILY: return rng.choice(FAMILY_LINES)
    s = f"{rng.choice(WHO)[0].upper()}{rng.choice(WHO)[1:]}"
    s = rng.choice(WHO)
    return f"{s[0].upper()}{s[1:]} {rng.choice(WHAT)}, {rng.choice(THEN)}."

def genres_for(title):
    if title in SCIFI: return ["Science Fiction"] + rng.sample(["Thriller", "Drama", "Adventure", "Mystery"], 1)
    if title in ACTION: return ["Action", "Thriller"] + ([] if rng.random() < .6 else ["Crime"])
    if title in HORROR: return ["Horror"] + rng.sample(["Thriller", "Mystery"], 1)
    if title in COMEDY: return ["Comedy"] + ([] if rng.random() < .5 else ["Romance"])
    if title in WESTERN: return ["Western"] + ([] if rng.random() < .5 else ["Drama"])
    if title in FAMILY: return ["Family", "Animation" if rng.random() < .6 else "Adventure"]
    return rng.choice([["Drama"], ["Drama", "Romance"], ["Mystery", "Drama"], ["Crime", "Drama"], ["Thriller"], ["Drama", "History"], ["Romance"], ["Adventure", "Drama"], ["Music", "Drama"], ["War", "Drama"]])

FIRST = "Ada Alba Amara Anika Arlo Asher Beatrix Bennett Calla Caspian Celia Dashiell Delphine Eamon Elodie Emrys Esme Felix Fenna Florian Greer Hale Harriet Idris Imogen Isaak Ivo Jonah Juniper Kasimir Keira Leander Lena Linus Lorcan Mabel Marius Maren Milo Mira Nadia Nico Odette Orla Otto Pax Penelope Quinn Rafe Rhea Rowan Saoirse Silas Soren Tamsin Teodor Thea Tobiah Una Viggo Wren Yara Zadie Callum Noor Ines Mateo Priya Kenji Amaru Sakura Oluwaseun Farah Anouk Björn Chiara Dmitri Esperanza Gideon Halima Ingrid Javier Kalani".split()
LAST = "Achterberg Ashdown Beaumont Blackwood Brennan Castellan Corvin Delacroix Draycott Ellery Fairweather Faraday Galloway Grayson Halloran Hartigan Hollis Ingram Jessup Kavanagh Kerrigan Lachance Lindqvist Marlowe Merriweather Montrose Nakamura Nightingale Okafor Oyelaran Pembrooke Quill Ravensworth Rosales Sandoval Sato Sinclair Solberg Thackeray Underhill Vance Varga Wainwright Whitlock Winslow Yardley Zeller Abernathy Bellamy Calloway Dunmore Everhart Fontaine Greenleaf Holloway Ishikawa Janssen Kowalski Lindgren Moreau Novak Osei Petrov Quintero Rahman Salazar Tanaka Umarov Valdez Westbrook Xu Yamamoto".split()
invented_people = []
used = set()
while len(invented_people) < 190:
    n = f"{rng.choice(FIRST)} {rng.choice(LAST)}"
    if n in used: continue
    used.add(n)
    by = rng.randint(1948, 2001)
    pid = person(n, born=f"{by}-{rng.randint(1,12):02d}-{rng.randint(1,28):02d}")
    people[pid]['bio'] = f"{n} is {rng.choice(['an actor', 'an actor and writer', 'a screen and stage actor', 'an actor and director'])} {rng.choice(['from Dublin', 'raised in Oslo', 'born in Lagos', 'from coastal Maine', 'raised in Kyoto', 'from Buenos Aires', 'born in Glasgow', 'from Vancouver Island', 'raised in Adelaide', 'from rural Ohio'])}, known for {rng.choice(['quiet, precise character work', 'a string of acclaimed independent dramas', 'comic timing honed in live theatre', 'intense genre performances', 'a long-running stage partnership'])}."
    invented_people.append(pid)
directors = invented_people[:36]
actors = invented_people[36:]

CERTS = ["PG", "PG", "PG-13", "R", "PG-13", "R", "PG-13", "NR"]
movies = []
def add_movie(m):
    movies.append(m)

for (title, year, mins, cert, gen, dirs, cast, syn, wiki, cat) in REAL:
    mid = 'm-' + slug(title) + f'-{year}'
    add_movie({'id': mid, 'title': title, 'year': year, 'runtime': mins, 'rating': cert, 'genres': gen,
               'directors': [person(d, wiki=d) for d in dirs], 'writers': [],
               'cast': [{'person': person(n, wiki=n), 'character': c} for n, c in cast],
               'overview': syn, 'art': {'kind': 'wiki', 'article': wiki, 'category': cat, 'tone': 'mono' if year < 1960 else 'warm'},
               'studio': rng.choice(["Public domain", "Criterion Vaults", "Archive Film"]), 'audience': round(rng.uniform(6.6, 8.6), 1), 'critic': rng.randint(78, 100),
               'library': 'lib-movies', 'real': True, 'tagline': None})
for (title, year, mins, cert, gen, dirs, syn, wiki, cat) in BLENDER:
    mid = 'm-' + slug(title) + f'-{year}'
    add_movie({'id': mid, 'title': title, 'year': year, 'runtime': mins, 'rating': cert, 'genres': gen,
               'directors': [person(d, wiki=None) for d in dirs], 'writers': [], 'cast': [],
               'overview': syn, 'art': {'kind': 'wiki', 'article': wiki, 'category': cat, 'tone': 'vivid'},
               'studio': 'Blender Studio', 'audience': round(rng.uniform(7.0, 8.4), 1), 'critic': rng.randint(80, 97),
               'library': 'lib-family' if cert in ('G', 'PG') else 'lib-movies', 'real': True, 'license': 'CC BY', 'tagline': None})

STUDIOS = ["Northlight Pictures", "Harbor & Vine", "Kestrel Films", "Blue Ember", "Meridian Studios", "Lantern House", "Saltmarsh Films", "Anchorage Pictures", "Greyfield", "Copperline", "Wildwood Pictures", "Tidewater Films"]
TAGLINES = ["Some roads only go one way.", "Everyone has a place they can't go back to.", "The tide always comes in.", "Nothing stays buried forever.", "One night. No way out.", "Home is a harder place to find.", "The truth has a long memory.", "Every map ends somewhere.", "Hold on to what you love.", "Listen closely.", "It started with a letter.", "Run while you can."]
for i, title in enumerate(INVENTED_MOVIES):
    title = title.strip()
    if not title: continue
    year = rng.choice([rng.randint(1978, 2026), rng.randint(2005, 2026), rng.randint(2014, 2026)])
    gen = genres_for(title)
    cert = 'G' if 'Family' in gen and rng.random() < .5 else ('PG' if 'Family' in gen else rng.choice(CERTS))
    mins = rng.randint(84, 148) if 'Family' not in gen else rng.randint(78, 104)
    lib = 'lib-family' if 'Family' in gen else 'lib-movies'
    cast = [{'person': p, 'character': f"{rng.choice(FIRST)} {rng.choice(LAST)}" if rng.random() < .7 else rng.choice(['The Stranger', 'Detective Hale', 'Captain', 'The Mother', 'Old Tom', 'Dr. Ames', 'Reporter', 'Sheriff', 'The Pilot', 'Narrator'])} for p in rng.sample(actors, rng.randint(4, 9))]
    add_movie({'id': 'm-' + slug(title), 'title': title, 'year': year, 'runtime': mins, 'rating': cert, 'genres': gen,
               'directors': [rng.choice(directors)], 'writers': rng.sample(directors, rng.randint(1, 2)), 'cast': cast,
               'overview': logline(title), 'art': {'kind': 'generated', 'style': rng.randint(0, 7)},
               'studio': rng.choice(STUDIOS), 'audience': round(rng.uniform(5.4, 8.9), 1), 'critic': rng.randint(35, 99), 'library': lib, 'real': False,
               'tagline': rng.choice(TAGLINES) if rng.random() < .7 else None})

# Collections (franchises and themed sets)
collections = [
 {'id': 'col-keaton', 'name': 'Buster Keaton', 'library': 'lib-movies', 'items': [m['id'] for m in movies if any(people[c['person']]['name'] == 'Buster Keaton' for c in m['cast'])], 'overview': 'The Great Stone Face: stunts, trains and deadpan genius.'},
 {'id': 'col-chaplin', 'name': 'Charlie Chaplin', 'library': 'lib-movies', 'items': [m['id'] for m in movies if any(people[c['person']]['name'] == 'Charlie Chaplin' for c in m['cast'])], 'overview': 'The Tramp, from the streets to the Klondike.'},
 {'id': 'col-expressionism', 'name': 'German Expressionism', 'library': 'lib-movies', 'items': [m['id'] for m in movies if m['title'] in ('Metropolis', 'Nosferatu', 'The Cabinet of Dr. Caligari', 'Faust', 'The Man Who Laughs', 'Sunrise: A Song of Two Humans')], 'overview': 'Shadows, angles and dread from the golden age of German silent film.'},
 {'id': 'col-noir', 'name': 'Film Noir Essentials', 'library': 'lib-movies', 'items': [m['id'] for m in movies if m['title'] in ('Detour', 'D.O.A.', 'Scarlet Street', 'The Stranger', 'The Hitch-Hiker', 'Kansas City Confidential', 'Suddenly')], 'overview': 'Rain-slick streets and bad decisions.'},
 {'id': 'col-blender', 'name': 'Blender Open Movies', 'library': 'lib-family', 'items': [m['id'] for m in movies if m.get('studio') == 'Blender Studio'], 'overview': 'Short films made in the open by the Blender community, shared under Creative Commons.'},
 {'id': 'col-vincent', 'name': 'Vincent Price Chillers', 'library': 'lib-movies', 'items': [m['id'] for m in movies if any(people[c['person']]['name'] == 'Vincent Price' for c in m['cast'])], 'overview': 'The velvet voice of horror.'},
 {'id': 'col-corman', 'name': 'Midnight Movies', 'library': 'lib-movies', 'items': [m['id'] for m in movies if m['title'] in ('Plan 9 from Outer Space', 'The Little Shop of Horrors', 'The Brain That Wouldn\'t Die', 'Night of the Living Dead', 'Carnival of Souls', 'Dementia 13', 'The Terror')], 'overview': 'Cult favorites for the late show.'},
 {'id': 'col-orbit', 'name': 'The Orbital Trilogy', 'library': 'lib-movies', 'items': [m['id'] for m in movies if m['title'] in ('Orbital', 'The Last Orbit', 'Deep Field')], 'overview': 'A crew, a station and the long way home.'},
 {'id': 'col-harbor', 'name': 'Harbor Lights Saga', 'library': 'lib-movies', 'items': [m['id'] for m in movies if m['title'] in ('Harbor Lights', 'Low Tide at Kessock', 'The Last Ferry', 'Anchor Point')], 'overview': 'Four films from one fishing town.'},
 {'id': 'col-heist', 'name': 'Heist Night', 'library': 'lib-movies', 'items': [m['id'] for m in movies if m['title'] in ('The Heist at Hollin Bank', 'Getaway Driver', 'The Fixer', 'The Extraction', 'The Honest Thief', 'The Forgery')], 'overview': 'Perfect plans, imperfect people.'},
]
for c in collections:
    for mid in c['items']:
        for m in movies:
            if m['id'] == mid: m.setdefault('collections', []).append(c['id'])

# ── TV shows and anime ────────────────────────────────────────────────────
SHOWS = """Harbor Station|The Night Desk|Kessler & Vane|Lowland Precinct|The Orchard|Signal & Noise|Glass City|The Keepers|North Shore Medical|Wild Coast|The Understudies|Copper County|Starboard|Hollow Creek|The Ministry of Small Things|Parallel|The Cartographers|Brightwater Bay|Salt & Stone|Dead Reckoning|The Long Winter|Midnight Kitchen|Frontier|The Archive|Ferrymen|Fault Line|The Lodge|Neon District|Tidewater|Lanterns|Garden Party|The Quiet Ones|Blackwood Manor|Riverside|Open Water|The Almanac Hour|Deep Cover|Moonrise Motel|The Weather Station|Fieldwork""".split('|')
ANIME = """Skyward Lantern|Blade of the Paper Moon|Starfield Academy|Mecha Garden|The Spirit Ferry|Neon Samurai 2099|Tidal Hearts|Wolf of the Eastern Wind|Cloud Kingdom|Summer Signal|Iron Petal|The Last Onmyoji|Café Constellation|Harbor Witch Delivery|Crystal Rail|Shadow Fox Detective Agency|Ramen Rhapsody|Paper Crane Squadron""".split('|')
NETWORKS = ["Harbor One", "Northlight TV", "Kestrel", "Blue Ember Network", "Meridian Plus", "Lantern", "Saltmarsh", "Copper Channel"]
ANIME_STUDIOS = ["Studio Hoshizora", "Kumo Animation", "Studio Kaze", "Aoi Pictures", "Tsubasa Works"]
EP_WORDS = "Arrival|The Offer|Crossing|Low Water|The Letter|Night Shift|Undertow|Aftermath|Signals|The Visitor|Fog|Homecoming|Old Debts|The Storm|Breakwater|First Light|Stranded|The Deal|Echoes|Last Call|Beacon|Fallout|The Map|Blind Spot|Salvage|Fire Drill|Open House|The Harvest|Detour|Thin Ice|The Reunion|Loose Ends|Point of No Return|The Wedding|Inventory|Ghosts|Paper Trail|Checkmate|The Long Night|Endgame|Pilot|Kindling|Riptide|Truth or Dare|Cold Open|The Trial|Silence|Relay|The Gift|Moving Day|Night Market|Overtime|Standoff|Snowed In|The Tape|Wildfire|Leverage|Half Measures|Undercurrent|The Bridge|Borrowed Time|Anchors".split('|')
ANIME_EP = "The Boy Who Heard the Stars|A Promise Under Cherry Blossoms|Enter the Iron Petal|The Festival of Lanterns|Rival on the Rooftop|Rain on the Old Shrine|The Tournament Begins|A Letter from the Sky|Training Day|Beneath the Moonlit Sea|The Stolen Relic|Farewell, Summer|Awakening|Sparks on the Rails|The Spirit's Request|Night of the Fox|Final Exam|Beyond the Clouds|The Ninth Gate|Friends and Rivals|Homecoming Festival|The Broken Blade|Storm Over the Capital|Where the Wind Goes".split('|')
SHOW_LINES = ["The staff of a small coastal police station handle everything from lost dogs to murder, and each other.", "Overnight editors at a struggling city newspaper chase the stories nobody else will touch.", "Two mismatched private investigators take the cases the big agencies turn down.", "A family-run orchard fights to survive developers, droughts and its own secrets.", "At a remote listening post, a young engineer picks up a signal that shouldn't exist.", "The residents of a glass-walled tower block can see into each other's lives, and each other's lies.", "Caretakers of a national archive discover that some records were never meant to be found.", "Doctors at an overstretched island hospital work miracles on a shoestring.", "A ranger service patrols a wild stretch of coast where the land and sea are both dangerous.", "A troupe of understudies waits in the wings of a legendary theatre for their big break.", "A reluctant sheriff returns to the mining county she swore she'd never go back to.", "A cast of misfits crews a secondhand cargo ship between tiny island ports.", "In a quiet mountain town, a missing hiker opens a door to decades of buried history.", "A tiny government department handles the smallest, strangest complaints in the country.", "The same group of friends lives two different lives, in two different worlds.", "A team of mapmakers charts the last unmapped corners of a vast northern wilderness.", "A cooking competition set in a midnight street market, where the ingredients are secret until the bell.", "Weather forecasters at a remote station become the only link to a snowbound valley."]
ANIME_LINES = ["A boy who can hear the stars enrols at a floating academy for pilots of the sky lanterns.", "A wandering swordswoman seeks the paper moon that holds her family's stolen memories.", "Students at an elite space academy compete to crew the first ship to the outer ring.", "A shy gardener inherits a giant robot that only moves when plants bloom.", "A girl ferries lost spirits across a river between worlds, one fare at a time.", "In a neon-lit future Tokyo, a masterless samurai takes jobs from androids and gangsters alike.", "A young witch opens a delivery service in a harbour town, with a broom that only flies downhill.", "A detective fox and her human assistant solve supernatural cases in the old capital."]

def make_series(title, kind, idx):
    sid = ('s-' if kind == 'tv' else 'a-') + slug(title)
    n_seasons = rng.choice([1, 2, 2, 3, 3, 4, 5]) if kind == 'tv' else rng.choice([1, 1, 2, 2, 3])
    start = rng.randint(2006, 2022)
    seasons = []
    ep_counter = 0
    for sn in range(1, n_seasons + 1):
        count = rng.choice([6, 8, 8, 10, 10, 12]) if kind == 'tv' else rng.choice([12, 12, 13, 24])
        eps = []
        words = ANIME_EP if kind == 'anime' else EP_WORDS
        for en in range(1, count + 1):
            ep_counter += 1
            t = 'Pilot' if (sn == 1 and en == 1 and kind == 'tv') else rng.choice(words)
            eps.append({'id': f'{sid}-s{sn}e{en}', 'season': sn, 'episode': en, 'absolute': ep_counter, 'title': t,
                        'runtime': rng.randint(42, 58) if kind == 'tv' else rng.randint(23, 25),
                        'airDate': f"{start + sn - 1}-{rng.randint(1,12):02d}-{rng.randint(1,28):02d}",
                        'overview': (rng.choice(["Tensions rise when", "Everything changes when", "An old friend returns just as", "The team is stretched thin when", "A storm cuts the town off as", "Secrets surface after"]) + ' ' + rng.choice(["a stranger arrives with a warning.", "a routine call turns dangerous.", "an unexpected inheritance divides the family.", "a body is found at the harbour.", "the power fails across the district.", "a rival makes an offer nobody can refuse.", "an old case is reopened.", "the festival brings unwelcome guests.", "a message arrives from the past.", "the last ferry fails to arrive."])),
                        'still': None, 'hasIntro': kind == 'tv' or True, 'hasCredits': True})
        seasons.append({'number': sn, 'title': f'Season {sn}', 'year': start + sn - 1, 'episodes': eps})
    if kind == 'tv' and rng.random() < .25:
        seasons.insert(0, {'number': 0, 'title': 'Specials', 'year': start, 'episodes': [{'id': f'{sid}-s0e1', 'season': 0, 'episode': 1, 'absolute': 0, 'title': 'Behind the Scenes', 'runtime': 24, 'airDate': f"{start+1}-12-20", 'overview': 'Cast and crew look back at making the show.', 'still': None, 'hasIntro': False, 'hasCredits': False}]})
    cast = [{'person': p, 'character': f"{rng.choice(FIRST)} {rng.choice(LAST)}"} for p in rng.sample(actors, rng.randint(4, 8))]
    lines = ANIME_LINES if kind == 'anime' else SHOW_LINES
    return {'id': sid, 'title': title, 'kind': kind, 'year': start, 'endYear': start + n_seasons - 1 if rng.random() < .5 else None,
            'rating': rng.choice(['TV-PG', 'TV-14', 'TV-14', 'TV-MA']) if kind == 'tv' else rng.choice(['TV-PG', 'TV-14', 'TV-Y7']),
            'genres': rng.choice([["Drama"], ["Crime", "Drama"], ["Mystery", "Drama"], ["Comedy"], ["Science Fiction", "Drama"], ["Reality"], ["Documentary"], ["Comedy", "Drama"], ["Thriller"]]) if kind == 'tv' else rng.sample(["Action", "Adventure", "Fantasy", "Comedy", "Slice of Life", "Science Fiction", "Romance", "Mystery"], 2),
            'network': rng.choice(NETWORKS) if kind == 'tv' else rng.choice(ANIME_STUDIOS),
            'overview': lines[idx % len(lines)], 'cast': cast, 'creators': [rng.choice(directors)],
            'seasons': seasons, 'audience': round(rng.uniform(6.8, 9.1), 1), 'library': 'lib-tv' if kind == 'tv' else 'lib-anime',
            'art': {'kind': 'generated', 'style': rng.randint(0, 7)}}

shows = [make_series(t, 'tv', i) for i, t in enumerate(SHOWS)] + [make_series(t, 'anime', i) for i, t in enumerate(ANIME)]

# ── Music ─────────────────────────────────────────────────────────────────
ARTISTS = """The Lantern Keepers|Maren Solberg|Harbor Static|Juniper & the Tides|Northern Lights Orchestra|Kasimir Vance|Glass Animals of Pell|Saltwater Choir|The Night Mail|Odette Moreau|Copper & Oak|Tidal Bloom|Linus Achterberg Trio|Violet Hour|Driftwood Radio|Esme Fairweather|Blue Mesa Band|The Cartographers|Kestrel Quartet|Neon Harbor|Soren Lindqvist|Wren & Rowan|Paper Kites of June|The Quiet Year|Midnight Ferry|Amara Okafor|Starling Society|Deep Field|Ember Choir|The Understory|Nadia Petrov|Holloway Brothers|Silver Creek Ramblers|Coastline|Isaak Grayson|Moth & Moon|Arcadia Strings|Fenna Janssen|Southbound Lanterns|The Weather Station|Orla Kavanagh|Lowlands|Solace Engine|Florian Sato|Heartwood|Cloudline|Tamsin Hollis|Brightwater Sound|The Almanac|Ivo Salazar""".split('|')
MUSIC_GENRES = ["Indie Folk", "Dream Pop", "Jazz", "Classical", "Electronic", "Americana", "Soul", "Ambient", "Rock", "Singer-Songwriter", "Synthpop", "Bluegrass", "Chamber Pop", "Lo-fi"]
ALBUM_WORDS = "Low Tide|Northern Hymns|Glasshouse|Salt & Light|Paper Moons|Long Division|Sleepwalker|Kindling|Harbor Songs|Afterglow|Blue Hour|The Quiet Hours|Meridian|Undertow|Wildflowers|Night Swimming|Cartography|Static Bloom|Evergreen|Signal Fires|Weather Report|Slow Rivers|Distance|Homeward|Lanterns|Ferris Wheel|Half Light|Constellations|Driftwood|Coastlines|Vespers|Tall Grass|Foxglove|Silver Linings|Parallel Lines|Morning Radio|Lost Frequencies|Golden Hour|Stillwater|Marigold|Starboard|Ember|Heavy Weather|Open Road|Sparrow|Tender Machines|Second Winds|Sea Glass|Midnight Garden|Echo Park".split('|')
TRACK_WORDS = "Lighthouse|Paper Boats|Carry Me Home|Tidewater|Slow Burn|Northern Line|Bright Morning|Undertow|Kerosene|Hold Still|Feather & Stone|Blue Light|Weathervane|Salt in the Air|The Long Way|Glass Heart|Evening Train|River Song|Wildfire|Lanterns|Harbor Lights|Morning Star|Satellites|Driftwood|Cold Water|Honey|Wolves|The Orchard|Firefly|Hummingbird|Echoes|Night Drive|Parallel|Hourglass|Old Radio|Winter Coat|Hollow|Gold Rush|Cartwheels|Sparrow|Ghost Town|Afterglow|Constellation|Porch Light|Anchors|Postcards|Moth|Signal|Seaside|Home Again|Summer Rain|Quiet Hours|Neon|Canyon|Low Tide|Paper Crowns|Starlight|Tin Can Telephone|Ferry Song|Last Dance".split('|')
LYRIC_LINES = ["The harbor lights are burning low", "I kept your letters in a paper boat", "We drove all night to find the sea", "Hold the lantern a little higher", "The radio plays our song again", "Every road leads back to you", "Salt in the air and sand in my shoes", "We were young and the summer was long", "Count the stars until the morning", "I hear the tide beneath the floor", "The orchard's quiet after rain", "Carry me home on the evening train", "Your voice is a signal in the static", "We built a house of glass and hope", "The river knows the way", "Stay a while, the night is kind", "Northern wind, carry my name", "Paper crowns and borrowed time", "I'll be the lighthouse on your shore", "Slow down, the world can wait"]
music_artists, albums = [], []
for i, name in enumerate(ARTISTS):
    aid = 'ar-' + slug(name)
    genre = rng.choice(MUSIC_GENRES)
    n_alb = rng.choice([1, 2, 2, 3, 3, 4])
    alb_ids = []
    for j in range(n_alb):
        title = rng.choice(ALBUM_WORDS)
        alid = f'al-{slug(name)}-{slug(title)}-{j}'
        year = rng.randint(1994, 2026)
        tracks = []
        disc2 = rng.random() < .08
        for t in range(1, rng.randint(8, 13) + 1):
            tt = rng.choice(TRACK_WORDS)
            lyr = None
            if rng.random() < .5:
                lines = rng.sample(LYRIC_LINES, 8)
                lyr = [{'at': k * rng.randint(9, 14) + 8, 'line': l} for k, l in enumerate(lines)]
            tracks.append({'id': f'{alid}-t{t}', 'number': t, 'disc': 2 if (disc2 and t > 6) else 1, 'title': tt, 'duration': rng.randint(150, 330), 'lyrics': lyr})
        albums.append({'id': alid, 'title': title, 'artist': aid, 'year': year, 'genre': genre, 'label': rng.choice(["Lantern Records", "Saltmarsh Sound", "North Pier", "Blue Ember Music", "Copperline Records", "Self-released"]), 'tracks': tracks, 'type': rng.choice(['Album', 'Album', 'Album', 'EP']), 'art': {'kind': 'generated', 'style': rng.randint(0, 5)}})
        alb_ids.append(alid)
    music_artists.append({'id': aid, 'name': name, 'genre': genre, 'albums': alb_ids, 'bio': f"{name} {rng.choice(['formed in a seaside town', 'began as a bedroom project', 'came together at music school', 'started busking on ferries', 'grew out of a late-night radio session'])} and {rng.choice(['has toured constantly ever since', 'built a devoted following one small venue at a time', 'records everything live to tape', 'is known for sprawling, cinematic live shows', 'writes about weather, distance and home'])}.", 'art': {'kind': 'photo'}})

# ── Audiobooks (public-domain classics, invented narrators) ───────────────
BOOKS = [("Pride and Prejudice", "Jane Austen", 1813, 61, "Classics"), ("Moby-Dick", "Herman Melville", 1851, 135, "Adventure"), ("Frankenstein", "Mary Shelley", 1818, 24, "Horror"), ("Dracula", "Bram Stoker", 1897, 27, "Horror"), ("The Time Machine", "H. G. Wells", 1895, 12, "Science Fiction"), ("Treasure Island", "Robert Louis Stevenson", 1883, 34, "Adventure"), ("Jane Eyre", "Charlotte Brontë", 1847, 38, "Classics"), ("Great Expectations", "Charles Dickens", 1861, 59, "Classics"), ("The Adventures of Sherlock Holmes", "Arthur Conan Doyle", 1892, 12, "Mystery"), ("Alice's Adventures in Wonderland", "Lewis Carroll", 1865, 12, "Children"), ("The Call of the Wild", "Jack London", 1903, 7, "Adventure"), ("Little Women", "Louisa May Alcott", 1868, 47, "Classics"), ("The War of the Worlds", "H. G. Wells", 1898, 27, "Science Fiction"), ("Heart of Darkness", "Joseph Conrad", 1899, 3, "Classics"), ("The Picture of Dorian Gray", "Oscar Wilde", 1890, 20, "Classics"), ("Wuthering Heights", "Emily Brontë", 1847, 34, "Classics"), ("The Secret Garden", "Frances Hodgson Burnett", 1911, 27, "Children"), ("Around the World in Eighty Days", "Jules Verne", 1872, 37, "Adventure"), ("Twenty Thousand Leagues Under the Seas", "Jules Verne", 1870, 47, "Science Fiction"), ("The Wind in the Willows", "Kenneth Grahame", 1908, 12, "Children"), ("A Christmas Carol", "Charles Dickens", 1843, 5, "Classics"), ("The Hound of the Baskervilles", "Arthur Conan Doyle", 1902, 15, "Mystery"), ("Anne of Green Gables", "L. M. Montgomery", 1908, 38, "Children"), ("The Invisible Man", "H. G. Wells", 1897, 28, "Science Fiction"), ("Sense and Sensibility", "Jane Austen", 1811, 50, "Classics"), ("The Jungle Book", "Rudyard Kipling", 1894, 7, "Children"), ("Peter Pan", "J. M. Barrie", 1911, 17, "Children"), ("The Strange Case of Dr Jekyll and Mr Hyde", "Robert Louis Stevenson", 1886, 10, "Horror"), ("Emma", "Jane Austen", 1815, 55, "Classics"), ("The Count of Monte Cristo", "Alexandre Dumas", 1844, 117, "Adventure")]
NARRATORS = ["Harriet Fairweather", "Callum Brennan", "Esme Lindqvist", "Otto Blackwood", "Priya Rahman", "Jonah Winslow", "Mabel Castellan", "Teodor Varga"]
SERIES = {"The Adventures of Sherlock Holmes": ("Sherlock Holmes", 1), "The Hound of the Baskervilles": ("Sherlock Holmes", 2), "Pride and Prejudice": ("Austen Collection", 2), "Sense and Sensibility": ("Austen Collection", 1), "Emma": ("Austen Collection", 3)}
books = []
for (title, author, year, chapters, genre) in BOOKS:
    bid = 'b-' + slug(title)
    chs = []
    for c in range(1, min(chapters, 60) + 1):
        chs.append({'number': c, 'title': f'Chapter {c}' if title not in ("The Adventures of Sherlock Holmes",) else ["A Scandal in Bohemia", "The Red-Headed League", "A Case of Identity", "The Boscombe Valley Mystery", "The Five Orange Pips", "The Man with the Twisted Lip", "The Adventure of the Blue Carbuncle", "The Adventure of the Speckled Band", "The Adventure of the Engineer's Thumb", "The Adventure of the Noble Bachelor", "The Adventure of the Beryl Coronet", "The Adventure of the Copper Beeches"][c - 1], 'duration': rng.randint(900, 2700)})
    ser = SERIES.get(title)
    books.append({'id': bid, 'title': title, 'author': author, 'narrator': rng.choice(NARRATORS), 'year': year, 'genre': genre, 'chapters': chs,
                  'overview': f"{title} by {author}, first published in {year}, read in full.", 'series': ser[0] if ser else None, 'seriesIndex': ser[1] if ser else None,
                  'publisher': rng.choice(["Lantern Audio", "Harbor Listening Library", "North Pier Audio"]), 'art': {'kind': 'generated', 'style': rng.randint(0, 5)}})

# ── Photos and home videos ────────────────────────────────────────────────
PHOTO_ALBUMS = [("Camera Roll", 32), ("Summer Trip", 24), ("Weekend Walks", 28), ("Favorites", 20), ("Road Trip", 30), ("Around Town", 18), ("Out and About", 26), ("Everyday", 22)]
photo_albums, n = [], 0
for name, count in PHOTO_ALBUMS:
    items = []
    for k in range(count):
        n += 1
        items.append({'id': f'ph-{n}', 'title': f'IMG_{4100 + n}', 'taken': f"20{rng.randint(19, 26)}-{rng.randint(1,12):02d}-{rng.randint(1,28):02d}T{rng.randint(7,21):02d}:{rng.randint(0,59):02d}", 'camera': rng.choice(["iPhone 16 Pro", "Fujifilm X100VI", "Sony α7 IV", "Pixel 9"]), 'portrait': rng.random() < .3, 'seed': n})
    photo_albums.append({'id': 'pa-' + slug(name), 'title': name, 'photos': items})
HOME_VIDEOS = ["Theo's First Steps", "Beach Day 2024", "Grandma's 80th", "Cabin Weekend", "Piano Recital", "Snowman Contest", "First Day of School", "Road Trip: Day 3", "Fireworks from the Pier", "Backyard Camping", "Kitchen Disaster (Pancakes)", "Soccer Final", "Maya's Graduation", "Northern Lights Timelapse", "The Great Bake Off at Home", "Halloween Parade", "Sailing Lesson", "Birthday Surprise", "Tree Planting Day", "Last Day at the Old House", "Holiday Morning", "Talent Show", "Bike Ride to the Lighthouse", "Science Fair Volcano"]
home_videos = [{'id': 'hv-' + slug(t), 'title': t, 'year': rng.randint(2018, 2026), 'runtime': rng.randint(2, 24), 'date': f"20{rng.randint(18, 26)}-{rng.randint(1,12):02d}-{rng.randint(1,28):02d}", 'overview': 'Filmed on a phone and kept for the family.'} for t in HOME_VIDEOS]

# ── Live channels (programmes are generated at runtime) ───────────────────
CHANNELS = [("2.1", "Harbor One", "news"), ("4.1", "Northlight", "general"), ("5.1", "KSTL 5", "general"), ("7.1", "Meridian News", "news"), ("9.1", "Classic Cinema", "movies"), ("11.1", "Kestrel Sports", "sports"), ("13.1", "Blue Ember", "drama"), ("15.1", "Lantern Kids", "kids"), ("17.1", "Weather Now", "weather"), ("19.1", "Saltmarsh Music", "music"), ("21.1", "Copper Comedy", "comedy"), ("23.1", "Wild Planet", "nature"), ("25.1", "History Vault", "history"), ("27.1", "Cookhouse", "food"), ("29.1", "Motor Nation", "sports"), ("31.1", "Anime Station", "anime"), ("33.1", "Sci-Fi Signal", "scifi"), ("35.1", "Mystery Theater", "mystery"), ("37.1", "Travel Coast", "travel"), ("39.1", "Public Square", "public"), ("41.1", "Late Show Live", "talk"), ("43.1", "Westerns 24/7", "westerns"), ("45.1", "Golf & Tennis", "sports"), ("47.1", "Retro TV", "retro"), ("49.1", "Home & Garden", "home"), ("51.1", "Science Hour", "science"), ("53.1", "World News 24", "news"), ("55.1", "Family Movie Night", "family")]
channels = [{'id': 'ch-' + slug(n), 'number': num, 'name': n, 'category': cat, 'group': {'news': 'News', 'weather': 'News', 'sports': 'Sports', 'kids': 'Kids & Family', 'family': 'Kids & Family', 'anime': 'Kids & Family', 'movies': 'Movies', 'westerns': 'Movies', 'scifi': 'Entertainment', 'mystery': 'Entertainment', 'drama': 'Entertainment', 'comedy': 'Entertainment', 'talk': 'Entertainment', 'retro': 'Entertainment'}.get(cat, 'Lifestyle'), 'source': 'src-hdhr' if i < 12 else 'src-m3u', 'hd': rng.random() < .8} for i, (num, n, cat) in enumerate(CHANNELS)]
library_channels = [{'id': 'lc-noir', 'number': '900', 'name': 'Noir After Dark', 'category': 'movies', 'group': 'Library Channels', 'source': 'library', 'hd': True, 'library': True, 'desc': 'Film noir from the Movies library, around the clock.'},
                    {'id': 'lc-keaton', 'number': '901', 'name': 'Silent Laughs', 'category': 'movies', 'group': 'Library Channels', 'source': 'library', 'hd': True, 'library': True, 'desc': 'Keaton, Chaplin and Lloyd on shuffle.'},
                    {'id': 'lc-harbor', 'number': '902', 'name': 'Harbor Station Marathon', 'category': 'drama', 'group': 'Library Channels', 'source': 'library', 'hd': True, 'library': True, 'desc': 'Every episode of Harbor Station in order.'},
                    {'id': 'lc-kids', 'number': '903', 'name': 'Saturday Cartoons', 'category': 'kids', 'group': 'Library Channels', 'source': 'library', 'hd': True, 'library': True, 'desc': 'Family library favorites.'}]

# Person images are real only for the real people; attach known-for.
for pid, p in people.items():
    p['credits'] = []
for m in movies:
    for c in m['cast']: people[c['person']]['credits'].append({'item': m['id'], 'role': 'cast', 'character': c['character']})
    for d in m['directors']: people[d]['credits'].append({'item': m['id'], 'role': 'crew', 'job': 'Director'})
    for w in m.get('writers', []): people[w]['credits'].append({'item': m['id'], 'role': 'crew', 'job': 'Writer'})
for s in shows:
    for c in s['cast']: people[c['person']]['credits'].append({'item': s['id'], 'role': 'cast', 'character': c['character']})
    for d in s['creators']: people[d]['credits'].append({'item': s['id'], 'role': 'crew', 'job': 'Creator'})
REAL_BIOS = {"Buster Keaton": ("1895-10-04", "1966-02-01", "Actor, director and stunt performer whose deadpan face and death-defying gags made him one of the great silent comedians."),
             "Charlie Chaplin": ("1889-04-16", "1977-12-25", "Comic actor and filmmaker who created the Tramp, the best-known character of the silent era."),
             "Harold Lloyd": ("1893-04-20", "1971-03-08", "Silent comedian famous for his everyman in round glasses and his thrill-comedy climbs."),
             "Cary Grant": ("1904-01-18", "1986-11-29", "English-American leading man celebrated for his charm and impeccable comic timing."),
             "Audrey Hepburn": ("1929-05-04", "1993-01-20", "Actress and humanitarian, an enduring icon of film and fashion."),
             "Vincent Price": ("1911-05-27", "1993-10-25", "Actor whose distinctive voice and gentlemanly menace made him a horror legend."),
             "Fritz Lang": ("1890-12-05", "1976-08-02", "Austrian-German-American director of expressionist epics and hard-edged American noir."),
             "F. W. Murnau": ("1888-12-28", "1931-03-11", "German director whose fluid camera and poetic images shaped silent cinema."),
             "Douglas Fairbanks": ("1883-05-23", "1939-12-12", "Swashbuckling star of silent adventure films and co-founder of United Artists."),
             "Edward G. Robinson": ("1893-12-12", "1973-01-26", "Actor who moved from gangster roles to some of noir's most memorable characters."),
             "Frank Sinatra": ("1915-12-12", "1998-05-14", "Singer and Academy Award-winning actor."),
             "John Wayne": ("1907-05-26", "1979-06-11", "Actor who defined the Hollywood western for four decades."),
             "Lon Chaney": ("1883-04-01", "1930-08-26", "The Man of a Thousand Faces, a master of make-up and physical transformation."),
             "Janet Gaynor": ("1906-10-06", "1984-09-14", "Actress and the first winner of the Academy Award for Best Actress."),
             "Carole Lombard": ("1908-10-06", "1942-01-16", "Leading comedienne of the screwball era."),
             "Bela Lugosi": ("1882-10-20", "1956-08-16", "Hungarian-American actor forever associated with horror.")}
for pid, p in people.items():
    if p['name'] in REAL_BIOS:
        b, d, bio = REAL_BIOS[p['name']]
        p.update({'born': b, 'died': d, 'bio': bio})
    elif p['real'] and not p.get('bio'):
        p['bio'] = f"{p['name']} is credited on {len(p['credits'])} {'title' if len(p['credits']) == 1 else 'titles'} in this library."

libraries = [
 {'id': 'lib-movies', 'name': 'Movies', 'kind': 'movie'},
 {'id': 'lib-tv', 'name': 'TV Shows', 'kind': 'tv'},
 {'id': 'lib-anime', 'name': 'Anime', 'kind': 'anime'},
 {'id': 'lib-family', 'name': 'Family', 'kind': 'movie'},
 {'id': 'lib-music', 'name': 'Music', 'kind': 'music'},
 {'id': 'lib-audiobooks', 'name': 'Audiobooks', 'kind': 'audiobook'},
 {'id': 'lib-photos', 'name': 'Photos', 'kind': 'photo'},
 {'id': 'lib-homevideos', 'name': 'Home Videos', 'kind': 'homevideo'},
]

# Copy passes that run after generation, with their own random source, so the
# main sequence (and every rendered image that depends on it) stays unchanged.
_copy = random.Random(4242)
_ADJ = "Paper|Silver|Quiet|Northern|Golden|Hollow|Borrowed|Late|Little|Second|Broken|Endless|Open|Painted|Distant|Burning|Summer|Winter|Midnight|Southern|Velvet|Crooked|Gentle|Empty|Wild|Faded|Electric|Lonely|Bright|Slow".split('|')
_NOUN = "Rivers|Ceilings|Letters|Harbors|Highways|Mirrors|Windows|Stations|Bridges|Horses|Gardens|Engines|Fences|Rooms|Signals|Porches|Maps|Kites|Tides|Houses|Hills|Streets|Birds|Pockets|Candles|Towns|Wires|Fields|Clocks|Doors".split('|')
_pool = list(dict.fromkeys(TRACK_WORDS + [f"{a} {n}" for a in _ADJ for n in _NOUN]))
_copy.shuffle(_pool)
_next = 0
for _al in albums:
    for _t in _al['tracks']:
        _t['title'] = _pool[_next % len(_pool)]
        _next += 1
for _hv in home_videos:
    _m = re.search(r'(20\d\d)', _hv['title'])
    if _m:
        _hv['date'] = _m.group(1) + _hv['date'][4:]
    _hv['year'] = int(_hv['date'][:4])

catalog = {'libraries': libraries, 'movies': movies, 'shows': shows, 'collections': collections, 'people': list(people.values()),
           'artists': music_artists, 'albums': albums, 'books': books, 'photoAlbums': photo_albums, 'homeVideos': home_videos,
           'channels': channels + library_channels}
out = os.path.join(ROOT, 'src', 'data', 'fixtures', 'catalogData.json')
os.makedirs(os.path.dirname(out), exist_ok=True)
json.dump(catalog, open(out, 'w'), ensure_ascii=False, separators=(',', ':'))
os.makedirs(os.path.join(ROOT, '.art-cache'), exist_ok=True)
json.dump(catalog, open(os.path.join(ROOT, '.art-cache', 'plan.json'), 'w'), ensure_ascii=False, indent=0)
print(f"movies {len(movies)} shows {len(shows)} episodes {sum(len(s['episodes']) for sh in shows for s in sh['seasons'])} people {len(people)} artists {len(music_artists)} albums {len(albums)} tracks {sum(len(a['tracks']) for a in albums)} books {len(books)} photos {n} channels {len(channels) + len(library_channels)}")
print('bytes', os.path.getsize(out))
