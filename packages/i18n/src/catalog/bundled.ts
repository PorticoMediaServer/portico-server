/**
 * Every translated catalogue, handed over at once. The apps import this: their bundle carries all
 * the languages anyway, and the first screen must already read the viewer's. The web loads each
 * language when it is needed instead (`loadCatalogue`).
 */
import {provideCatalogue} from './index.ts';
import {messages as fr} from './translated/fr.ts';
import {messages as es} from './translated/es.ts';
import {messages as de} from './translated/de.ts';
import {messages as ru} from './translated/ru.ts';
import {messages as ptBR} from './translated/pt-BR.ts';
import {messages as it} from './translated/it.ts';
import {messages as nl} from './translated/nl.ts';
import {messages as pl} from './translated/pl.ts';
import {messages as sv} from './translated/sv.ts';
import {messages as da} from './translated/da.ts';
import {messages as nb} from './translated/nb.ts';
import {messages as cs} from './translated/cs.ts';
import {messages as tr} from './translated/tr.ts';
import {messages as uk} from './translated/uk.ts';
import {messages as zhHans} from './translated/zh-Hans.ts';
import {messages as zhHant} from './translated/zh-Hant.ts';
import {messages as ja} from './translated/ja.ts';
import {messages as ko} from './translated/ko.ts';
import {messages as frCA} from './translated/fr-CA.ts';
import {messages as es419} from './translated/es-419.ts';
import {messages as ptPT} from './translated/pt-PT.ts';
import {messages as el} from './translated/el.ts';
import {messages as hu} from './translated/hu.ts';
import {messages as ro} from './translated/ro.ts';
import {messages as fi} from './translated/fi.ts';
import {messages as id} from './translated/id.ts';
import {messages as vi} from './translated/vi.ts';
import {messages as th} from './translated/th.ts';
import {messages as hi} from './translated/hi.ts';

provideCatalogue('fr', fr);
provideCatalogue('es', es);
provideCatalogue('de', de);
provideCatalogue('ru', ru);
provideCatalogue('pt-BR', ptBR);
provideCatalogue('it', it);
provideCatalogue('nl', nl);
provideCatalogue('pl', pl);
provideCatalogue('sv', sv);
provideCatalogue('da', da);
provideCatalogue('nb', nb);
provideCatalogue('cs', cs);
provideCatalogue('tr', tr);
provideCatalogue('uk', uk);
provideCatalogue('zh-Hans', zhHans);
provideCatalogue('zh-Hant', zhHant);
provideCatalogue('ja', ja);
provideCatalogue('ko', ko);
provideCatalogue('fr-CA', frCA);
provideCatalogue('es-419', es419);
provideCatalogue('pt-PT', ptPT);
provideCatalogue('el', el);
provideCatalogue('hu', hu);
provideCatalogue('ro', ro);
provideCatalogue('fi', fi);
provideCatalogue('id', id);
provideCatalogue('vi', vi);
provideCatalogue('th', th);
provideCatalogue('hi', hi);
