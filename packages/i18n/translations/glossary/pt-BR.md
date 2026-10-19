# Glossário pt-BR (português brasileiro) — Portico

Tradução automática inicial (state `machine` em todas as 5.357 mensagens);
precisa de revisão por falante nativo antes de publicar.
Tom: informal no padrão de apps de streaming no Brasil (Netflix/Plex):
o espectador é tratado por **“você”** (nunca “tu”, nunca “o senhor”).

## Formas de tratamento e gramática

- Segunda pessoa informal: “Verifique seus dados”, “Esqueceu o PIN?”,
  “Você continua conectado”. Imperativo direto (“Entre”, “Tente novamente”,
  “Escolha”).
- Gerúndio brasileiro normal (“Continuar assistindo”, “Carregando…”).
- Reticências sempre com o caractere único `…`, nunca `...`.
- Aspas de citação: `“ ”` como no original. Setas `›` e `·` mantidas.
- Títulos de botões e abas em estilo de frase curta, sem ponto final;
  frases de ajuda e erros com ponto final.
- Sem apóstrofo ASCII em parte alguma (regra do ICU); o português
  praticamente não usa apóstrofo, então nenhuma mensagem precisa dele.

## Termos centrais (inglês → termo escolhido)

| Inglês | pt-BR | Nota |
| --- | --- | --- |
| Settings | Configurações | |
| Preferences | Preferências | página de preferências do perfil |
| Sign in (verb) | Entrar | botão/ação |
| Sign-in (noun), login | login | “O login não foi concluído” |
| Sign out | Sair | “Sair deste dispositivo” |
| Library / Libraries | Biblioteca / Bibliotecas | |
| Home | Início | |
| Search (verb/noun) | Buscar / Busca | “Buscar”, “Sua busca” |
| My List | Minha Lista | |
| Saved | Salvos | seção “Salvos” |
| Favorites | Favoritos | |
| Continue Watching | Continuar assistindo | |
| Watch Together | Assistir Juntos | nome do recurso |
| Watchlist | Minha Lista | |
| Play | Reproduzir | cobre vídeo e áudio |
| Resume | Continuar | |
| Shuffle | Aleatório | adjetivo/ação conforme contexto |
| Subtitles / Captions | Legendas | |
| Episode / Season | Episódio / Temporada | |
| Show (TV) | Série | |
| Movie | Filme | |
| Track (music) | Faixa | “Número da faixa” |
| Album / Artist / Song | Álbum / Artista / Música | |
| Audiobook | Audiolivro | |
| Downloads | Downloads | substantivo mantido |
| Download (verb) | Baixar | |
| Profile | Perfil | |
| Account | Conta | |
| Portico Account | Portico Account | nome de produto, intraduzível |
| Portico / Portico Server | Portico / Portico Server | nomes de produto |
| Owner | Dono | “dono do servidor” |
| Member | Membro | |
| Server / Servers | Servidor / Servidores | |
| Remote access | Acesso remoto | |
| Live TV | TV ao vivo | |
| Guide | Guia | guia de programação |
| Tuner | Sintonizador | |
| Recording(s) | Gravação / Gravações | DVR |
| Channel | Canal | |
| Playlist | Playlist | mantido (uso corrente no Brasil) |
| Collection | Coleção | |
| Refresh | Atualizar | |
| Retry / Try again | Tentar novamente | botão curto: “Tentar de novo” quando couber; texto corrido usa “Tentar novamente” |
| Dismiss | Dispensar | avisos |
| Discard | Descartar | ações destrutivas leves |
| Cancel | Cancelar | |
| Undo | Desfazer | |
| Save | Salvar | |
| Delete | Excluir | destrutivo (“Excluir conta”) |
| Remove | Remover | não destrutivo (“Remover da fila”) |
| Loading | Carregando | |
| Unavailable | Indisponível | |
| Offline | Off-line | com hífen, padrão BR |
| Online | On-line | |
| Quality | Qualidade | |
| Original (quality) | Original | |
| Automatic | Automática | concorda com “qualidade” |
| Conversion / Convert | Conversão / Converter | transcodificação |
| Buffer / Buffering | Buffer / Carregando | |
| Rating (user) | Avaliação | “Sua avaliação” |
| Content rating | Classificação indicativa | sistema BR (L/10/12/14/16/18) |
| Studio | Estúdio | |
| Network (broadcaster) | Emissora | |
| Cast / Crew | Elenco / Equipe | |
| Director / Writer | Diretor / Roteirista | |
| Notifications | Notificações | |
| History | Histórico | |
| Devices | Dispositivos | |
| Phone / Tablet | Celular / Tablet | “celular”, não “telefone” |
| TV | TV | |
| PIN | PIN | |
| Password | Senha | |
| Two-step verification | Verificação em duas etapas | |
| Authenticator app | App autenticador | sem hífen, uso corrente |
| Recovery codes | Códigos de recuperação | |
| Invitation / Invite | Convite / Convidar | |
| Quick Connect | Conexão Rápida | nome do recurso |
| Library Channels | Canais da Biblioteca | nome do recurso |
| Metadata | Metadados | masculino plural |
| Artwork | Arte / Artes | |
| Lyrics | Letra | |
| Sleep timer | Timer (de sono) | “Timer”, “Timer de sono” |
| Playback speed | Velocidade | |
| HDR | HDR | sigla mantida |
| Dolby Vision | Dolby Vision | marca mantida |
| Picture in Picture | Picture in Picture | nome de recurso da plataforma |
| AirPlay / Google Cast | AirPlay / Google Cast | marcas mantidas |
| mpv | mpv | marca mantida |
| FFmpeg / WebDAV / rclone / M3U / XMLTV | mantidos | termos técnicos |
| Webhook(s) | Webhook(s) | mantido; “Link do convite” para invite link |
| API key / Token | Chave de API / Token | |
| Backup | Backup | |
| Folder | Pasta | “Escolher pasta” |
| File | Arquivo | |
| Storage | Armazenamento | |
| Trash-like states | Mantido / Excluído | “Mantido” para kept |
| Watched / Unwatched | Assistido / Não assistido | vídeo |
| Played / Unplayed | Reproduzido / Não reproduzido | música |
| Finished | Terminado / Terminada | concorda com o substantivo |
| Missing (title) | Ausente | título sem arquivos |
| Unmatched | Sem correspondência | identificação de metadados |
| Match (metadata) | Correspondência / Identificação | verbo: “Identificar” |
| Report a problem | Relatar um problema | |
| Feedback | Comentários | |

## Decisões que o revisor deve conferir

- **Play → “Reproduzir”** (não “Assistir”/“Tocar”): cobre filmes, músicas
  e episódios com um verbo só, como o Plex em PT. “Assistir” aparece só
  onde o original diz “watch” (“Continuar assistindo”, “Assistir Juntos”).
- **Watch Together → “Assistir Juntos”**: nome do recurso, usado em todas
  as chaves `together.*`, `player.watchTogether`, `feature.*`, `limits.*`.
- **Sign in → “Entrar”** mas o substantivo vira **“login”**
  (“O login não foi concluído”). “Acesse” foi evitado.
- **Library Channels → “Canais da Biblioteca”**; canais comuns → “Canais”.
- **Quick Connect → “Conexão Rápida”**.
- **S{season} · E{episode} → T{season} · E{episode}** (Temporada/Episódio).
- **“Saved › Collections” → “Salvos › Coleções”**.
- **Gênero em selects**: ramos de `error.title.*` concordam com cada
  substantivo (“O Início não pôde ser carregado”, “Esta biblioteca não
  pôde ser carregada”, “Os downloads não puderam ser carregados”).
- **“Your Portico Account …”** mantém o nome do produto:
  “Sua Portico Account…”, “na sua Portico Account”.
- **Plural `=0` preservado** em `recordings.matches`,
  `web.metadata.candidateCount` e equivalentes `web.dvr.*`;
  categorias `one`/`other` em todos os plurais (pt-BR só precisa dessas).
- **Esqueleto numérico** `{height, number, ::group-off}p` copiado exato.
- **Exemplo de código rclone** (`[drive]\ntype = drive\n…`) mantido.
- **URLs** `getportico.tv/terms` e `getportico.tv/privacy` mantidas.
- **Nomes de dispositivos** iPhone/iPad/Apple TV/Android TV/Fire TV
  mantidos; genéricos viram “celular” e “tablet”.
- Pontos fracos prováveis: termos de DVR/guia (“listing” → “listing de
  guia”, “padding” → “folga”, “slate” → “cartaz”), mensagens longas do
  console do servidor e textos jurídicos de licenças — merecem revisão
  atenta de um nativo.
