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

provideCatalogue('fr', fr);
provideCatalogue('es', es);
provideCatalogue('de', de);
provideCatalogue('ru', ru);
