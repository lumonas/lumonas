import type {
  CatalogApp,
  DockerContainer,
  DockerDeployment,
  DockerImage,
  DockerStack,
  DockerVolume,
  LogLine,
} from '@/api/types'

const now = Date.now()
const hoursAgo = (h: number) => new Date(now - h * 3_600_000).toISOString()
const daysAgo = (d: number) => new Date(now - d * 86_400_000).toISOString()
const GB = 1_000_000_000
const MB = 1_000_000

export const catalog: CatalogApp[] = [
  {
    id: 'jellyfin',
    name: 'Jellyfin',
    category: 'Media',
    tagline: 'Stream your movies, shows and music',
    description:
      'Free software media system that puts you in control of your media. Stream to any device from your own server.',
    accent: 'info',
    upstream: 'https://jellyfin.org',
    image: 'jellyfin/jellyfin:10.10.6',
    ports: [8096],
    popular: true,
    form: [
      { id: 'PORT', label: 'Web port', type: 'port', defaultValue: '8096', required: true },
      {
        id: 'MEDIA',
        label: 'Media library',
        type: 'storage_ref',
        containerPath: '/media',
        defaultResource: 'share-media',
        required: true,
        description: 'Where Jellyfin looks for movies and shows',
      },
      { id: 'TZ', label: 'Timezone', type: 'timezone' },
    ],
  },
  {
    id: 'immich',
    name: 'Immich',
    category: 'Photos',
    tagline: 'Self-hosted photo and video backup',
    description:
      'High performance photo and video backup solution directly from your mobile phone.',
    accent: 'primary',
    upstream: 'https://immich.app',
    image: 'ghcr.io/immich-app/immich-server:v1.116.0',
    ports: [2283],
    popular: true,
    form: [
      { id: 'PORT', label: 'Web port', type: 'port', defaultValue: '2283', required: true },
      {
        id: 'UPLOAD_LOCATION',
        label: 'Upload storage',
        type: 'storage_ref',
        containerPath: '/usr/src/app/upload',
        defaultResource: 'share-photos',
        required: true,
      },
      {
        id: 'DB_PASSWORD',
        label: 'Database password',
        type: 'secret',
        required: true,
        description: 'Generated automatically — stored encrypted in the LumoNAS secret store',
      },
      { id: 'TZ', label: 'Timezone', type: 'timezone' },
    ],
  },
  {
    id: 'plex',
    name: 'Plex',
    category: 'Media',
    tagline: 'Media streaming with remote access',
    description: 'Organize and stream your media to any device, with optional Plex Pass features.',
    accent: 'attention',
    upstream: 'https://plex.tv',
    image: 'plexinc/pms-docker:1.41.0',
    ports: [32400],
    form: [
      { id: 'PORT', label: 'Web port', type: 'port', defaultValue: '32400', required: true },
      {
        id: 'MEDIA',
        label: 'Media library',
        type: 'storage_ref',
        containerPath: '/data',
        defaultResource: 'share-media',
        required: true,
      },
    ],
  },
  {
    id: 'homeassistant',
    name: 'Home Assistant',
    category: 'Home',
    tagline: 'Open source home automation',
    description:
      'Powerful smart home hub that works with thousands of devices. Needs host network access.',
    accent: 'warning',
    upstream: 'https://home-assistant.io',
    image: 'ghcr.io/home-assistant/home-assistant:stable',
    ports: [8123],
    form: [
      { id: 'TZ', label: 'Timezone', type: 'timezone' },
      {
        id: 'CONFIG',
        label: 'Configuration storage',
        type: 'storage_ref',
        containerPath: '/config',
        defaultResource: 'apps',
        required: true,
      },
    ],
  },
  {
    id: 'pihole',
    name: 'Pi-hole',
    category: 'Network',
    tagline: 'Network-wide ad blocking',
    description: 'A DNS sinkhole that protects your devices from ads and trackers at the network level.',
    accent: 'success',
    upstream: 'https://pi-hole.net',
    image: 'pihole/pihole:v6.0',
    ports: [80],
    form: [
      {
        id: 'WEBPASSWORD',
        label: 'Admin password',
        type: 'secret',
        required: true,
        description: 'Stored encrypted in the LumoNAS secret store',
      },
    ],
  },
  {
    id: 'nextcloud',
    name: 'Nextcloud',
    category: 'Files',
    tagline: 'Your personal cloud',
    description: 'File sync and share platform with calendars, contacts and hundreds of apps.',
    accent: 'info',
    upstream: 'https://nextcloud.com',
    image: 'nextcloud:29.0.4-apache',
    ports: [8080],
    form: [
      { id: 'PORT', label: 'Web port', type: 'port', defaultValue: '8080', required: true },
      {
        id: 'NC_DATA',
        label: 'Data storage',
        type: 'storage_ref',
        containerPath: '/var/www/html/data',
        defaultResource: 'share-cloud',
        required: true,
      },
      {
        id: 'TRUSTED_DOMAINS',
        label: 'Trusted domains',
        type: 'text',
        defaultValue: 'lumo-one.local',
        description: 'Comma-separated hostnames allowed to access Nextcloud',
      },
      { id: 'TZ', label: 'Timezone', type: 'timezone' },
    ],
  },
  {
    id: 'sonarr',
    name: 'Sonarr',
    category: 'Downloads',
    tagline: 'TV show collection manager',
    description: 'Smart PVR for newsgroup and bittorrent users. Tracks, grabs and renames TV episodes.',
    accent: 'primary',
    upstream: 'https://sonarr.tv',
    image: 'linuxserver/sonarr:4.0.11',
    ports: [8989],
    form: [
      { id: 'PORT', label: 'Web port', type: 'port', defaultValue: '8989', required: true },
      {
        id: 'TV',
        label: 'TV library',
        type: 'storage_ref',
        containerPath: '/tv',
        defaultResource: 'share-media',
        required: true,
      },
    ],
  },
  {
    id: 'radarr',
    name: 'Radarr',
    category: 'Downloads',
    tagline: 'Movie collection manager',
    description: 'Movie collection manager for usenet and torrent users, with calendar and quality profiles.',
    accent: 'attention',
    upstream: 'https://radarr.video',
    image: 'linuxserver/radarr:5.15.1',
    ports: [7878],
    form: [
      { id: 'PORT', label: 'Web port', type: 'port', defaultValue: '7878', required: true },
      {
        id: 'MOVIES',
        label: 'Movie library',
        type: 'storage_ref',
        containerPath: '/movies',
        defaultResource: 'share-media',
        required: true,
      },
    ],
  },
  {
    id: 'grafana',
    name: 'Grafana',
    category: 'Monitoring',
    tagline: 'Dashboards for everything',
    description: 'Query, visualize and alert on metrics from any data source.',
    accent: 'warning',
    upstream: 'https://grafana.com',
    image: 'grafana/grafana:11.3.0',
    ports: [3000],
    form: [{ id: 'PORT', label: 'Web port', type: 'port', defaultValue: '3000', required: true }],
  },
  {
    id: 'paperless',
    name: 'Paperless-ngx',
    category: 'Documents',
    tagline: 'Scan, index and archive documents',
    description: 'Transform physical documents into a searchable archive with OCR and tags.',
    accent: 'success',
    upstream: 'https://docs.paperless-ngx.com',
    image: 'paperlessngx/paperless-ngx:2.13.2',
    ports: [8000],
    form: [
      { id: 'PORT', label: 'Web port', type: 'port', defaultValue: '8000', required: true },
      {
        id: 'PAPERLESS_DATA',
        label: 'Document storage',
        type: 'storage_ref',
        containerPath: '/data',
        defaultResource: 'share-documents',
        required: true,
      },
      { id: 'TZ', label: 'Timezone', type: 'timezone' },
    ],
  },
]

const JELLYFIN_COMPOSE = `services:
  jellyfin:
    image: jellyfin/jellyfin:10.10.6
    container_name: jellyfin
    environment:
      - TZ=\${TZ}
    volumes:
      - /srv/lumonas/apps/jellyfin/config:/config
      - \${MEDIA}:/media
    ports:
      - 8096:8096
    restart: unless-stopped`

const IMMICH_COMPOSE = `services:
  immich-server:
    image: ghcr.io/immich-app/immich-server:v1.116.0
    container_name: immich-server
    volumes:
      - \${UPLOAD_LOCATION}:/usr/src/app/upload
      - /etc/localtime:/etc/localtime:ro
    ports:
      - 2283:2283
    depends_on:
      - immich-db
      - immich-redis
    restart: unless-stopped

  immich-machine-learning:
    image: ghcr.io/immich-app/immich-machine-learning:v1.116.0
    container_name: immich-machine-learning
    volumes:
      - /srv/lumonas/apps/immich/ml-cache:/cache
    restart: unless-stopped

  immich-db:
    image: tensorchord/pgvecto-rs:pg14-v0.2.0
    container_name: immich-db
    environment:
      - POSTGRES_PASSWORD=\${DB_PASSWORD}
      - POSTGRES_USER=postgres
    volumes:
      - /srv/lumonas/apps/immich/db:/var/lib/postgresql/data
    restart: unless-stopped

  immich-redis:
    image: redis:7.4-alpine
    container_name: immich-redis
    restart: unless-stopped`

const HOMEASSISTANT_COMPOSE = `services:
  homeassistant:
    image: ghcr.io/home-assistant/home-assistant:stable
    container_name: homeassistant
    network_mode: host
    privileged: true
    devices:
      - /dev/ttyUSB0:/dev/ttyUSB0
    volumes:
      - /srv/lumonas/apps/homeassistant/config:/config
      - /etc/localtime:/etc/localtime:ro
    environment:
      - TZ=\${TZ}
    restart: unless-stopped`

const PIHOLE_COMPOSE = `services:
  pihole:
    image: pihole/pihole:v6.0
    container_name: pihole
    environment:
      - TZ=\${TZ}
      - FTLCONF_webserver_api_password=\${WEBPASSWORD}
    volumes:
      - /srv/lumonas/apps/pihole/etc-pihole:/etc/pihole
    ports:
      - 80:80/tcp
      - 53:53/tcp
      - 53:53/udp
    restart: unless-stopped`

const SONARR_COMPOSE = `services:
  sonarr:
    image: linuxserver/sonarr:4.0.11
    container_name: sonarr
    environment:
      - TZ=\${TZ}
    volumes:
      - /srv/lumonas/apps/sonarr/config:/config
      - \${TV}:/tv
      - \${DOWNLOADS}:/downloads
    ports:
      - 8989:8989
    restart: unless-stopped`

const RADARR_COMPOSE = `services:
  radarr:
    image: linuxserver/radarr:5.15.1
    container_name: radarr
    environment:
      - TZ=\${TZ}
    volumes:
      - /srv/lumonas/apps/radarr/config:/config
      - \${MOVIES}:/movies
      - \${DOWNLOADS}:/downloads
    ports:
      - 7878:7878
    restart: unless-stopped`

const NEXTCLOUD_COMPOSE = `services:
  nextcloud-app:
    image: nextcloud:29.0.4-apache
    container_name: nextcloud-app
    environment:
      - POSTGRES_HOST=nextcloud-db
      - POSTGRES_PASSWORD=\${DB_PASSWORD}
      - NEXTCLOUD_TRUSTED_DOMAINS=\${TRUSTED_DOMAINS}
    volumes:
      - /srv/lumonas/apps/nextcloud/html:/var/www/html
      - \${NC_DATA}:/var/www/html/data
    ports:
      - 8080:80
    depends_on:
      - nextcloud-db
      - nextcloud-redis
    restart: unless-stopped

  nextcloud-db:
    image: postgres:16-alpine
    container_name: nextcloud-db
    environment:
      - POSTGRES_PASSWORD=\${DB_PASSWORD}
    volumes:
      - /srv/lumonas/apps/nextcloud/db:/var/lib/postgresql/data
    restart: unless-stopped

  nextcloud-redis:
    image: redis:7.4-alpine
    container_name: nextcloud-redis
    restart: unless-stopped`

export const stacks: DockerStack[] = [
  {
    id: 'stack-jellyfin',
    name: 'jellyfin',
    catalogId: 'jellyfin',
    category: 'Media',
    status: 'healthy',
    state: 'running',
    images: ['jellyfin/jellyfin:10.10.6'],
    composeYaml: JELLYFIN_COMPOSE,
    env: [
      { name: 'TZ', value: 'Europe/Warsaw', scope: 'builtin' },
      { name: 'MEDIA', value: 'Media → /', scope: 'global' },
    ],
    storage: [
      { containerPath: '/config', resourceId: 'apps', resourceLabel: 'Apps SSD / jellyfin' },
      { containerPath: '/media', resourceId: 'share-media', resourceLabel: 'Media (share)' },
    ],
    ports: [{ host: 8096, container: 8096, label: 'Web UI' }],
    risks: [],
    cpuPercent: 4,
    ramUsedBytes: 1.2 * GB,
    restarts: 0,
    lastDeploy: daysAgo(12),
    updateAvailable: { current: '10.10.6', latest: '10.10.7' },
    backup: { strategy: 'stop-backup', lastBackupAt: hoursAgo(26), appdataSizeBytes: 2.1 * GB },
    recoveryCoverage: 94,
  },
  {
    id: 'stack-immich',
    name: 'immich',
    catalogId: 'immich',
    category: 'Photos',
    status: 'healthy',
    state: 'running',
    images: [
      'ghcr.io/immich-app/immich-server:v1.116.0',
      'ghcr.io/immich-app/immich-machine-learning:v1.116.0',
      'tensorchord/pgvecto-rs:pg14-v0.2.0',
      'redis:7.4-alpine',
    ],
    composeYaml: IMMICH_COMPOSE,
    env: [
      { name: 'TZ', value: 'Europe/Warsaw', scope: 'builtin' },
      { name: 'UPLOAD_LOCATION', value: 'Photos → /', scope: 'global' },
      { name: 'DB_PASSWORD', value: 'secret:immich-db', scope: 'secret' },
    ],
    storage: [
      { containerPath: '/usr/src/app/upload', resourceId: 'share-photos', resourceLabel: 'Photos (share)' },
      { containerPath: '/cache', resourceId: 'apps', resourceLabel: 'Apps SSD / immich/ml-cache' },
      { containerPath: '/var/lib/postgresql/data', resourceId: 'apps', resourceLabel: 'Apps SSD / immich/db' },
    ],
    ports: [{ host: 2283, container: 2283, label: 'Web UI' }],
    risks: [],
    cpuPercent: 3,
    ramUsedBytes: 2.8 * GB,
    restarts: 1,
    lastDeploy: daysAgo(6),
    backup: { strategy: 'stop-backup', lastBackupAt: hoursAgo(3), appdataSizeBytes: 4.6 * GB },
    recoveryCoverage: 88,
  },
  {
    id: 'stack-homeassistant',
    name: 'homeassistant',
    catalogId: 'homeassistant',
    category: 'Home',
    status: 'attention',
    state: 'running',
    images: ['ghcr.io/home-assistant/home-assistant:stable'],
    composeYaml: HOMEASSISTANT_COMPOSE,
    env: [{ name: 'TZ', value: 'Europe/Warsaw', scope: 'builtin' }],
    storage: [{ containerPath: '/config', resourceId: 'apps', resourceLabel: 'Apps SSD / homeassistant' }],
    ports: [{ host: 8123, container: 8123, label: 'Web UI' }],
    risks: ['host_network', 'privileged', 'devices'],
    cpuPercent: 6,
    ramUsedBytes: 940 * MB,
    restarts: 0,
    lastDeploy: daysAgo(30),
    backup: { strategy: 'crash-consistent', lastBackupAt: daysAgo(2), appdataSizeBytes: 640 * MB },
    recoveryCoverage: 76,
  },
  {
    id: 'stack-pihole',
    name: 'pihole',
    catalogId: 'pihole',
    category: 'Network',
    status: 'critical',
    state: 'unhealthy',
    images: ['pihole/pihole:v6.0'],
    composeYaml: PIHOLE_COMPOSE,
    env: [
      { name: 'TZ', value: 'Europe/Warsaw', scope: 'builtin' },
      { name: 'WEBPASSWORD', value: 'secret:pihole-web', scope: 'secret' },
    ],
    storage: [{ containerPath: '/etc/pihole', resourceId: 'apps', resourceLabel: 'Apps SSD / pihole' }],
    ports: [
      { host: 80, container: 80, label: 'Web UI' },
      { host: 53, container: 53, label: 'DNS' },
    ],
    risks: [],
    cpuPercent: 1,
    ramUsedBytes: 180 * MB,
    restarts: 3,
    lastDeploy: daysAgo(2),
    backup: { strategy: 'stop-backup', appdataSizeBytes: 42 * MB },
    recoveryCoverage: 0,
  },
  {
    id: 'stack-sonarr',
    name: 'sonarr',
    catalogId: 'sonarr',
    category: 'Downloads',
    status: 'offline',
    state: 'stopped',
    images: ['linuxserver/sonarr:4.0.11'],
    composeYaml: SONARR_COMPOSE,
    env: [
      { name: 'TZ', value: 'Europe/Warsaw', scope: 'builtin' },
      { name: 'TV', value: 'Media → /TV', scope: 'global' },
      { name: 'DOWNLOADS', value: 'Backups → /downloads', scope: 'global' },
    ],
    storage: [
      { containerPath: '/config', resourceId: 'apps', resourceLabel: 'Apps SSD / sonarr' },
      { containerPath: '/tv', resourceId: 'share-media', resourceLabel: 'Media (share)' },
    ],
    ports: [{ host: 8989, container: 8989, label: 'Web UI' }],
    risks: [],
    cpuPercent: 0,
    ramUsedBytes: 0,
    restarts: 0,
    lastDeploy: daysAgo(20),
    backup: { strategy: 'stop-backup', lastBackupAt: daysAgo(5), appdataSizeBytes: 310 * MB },
    recoveryCoverage: 81,
  },
  {
    id: 'stack-radarr',
    name: 'radarr',
    catalogId: 'radarr',
    category: 'Downloads',
    status: 'healthy',
    state: 'running',
    images: ['linuxserver/radarr:5.15.1'],
    composeYaml: RADARR_COMPOSE,
    env: [
      { name: 'TZ', value: 'Europe/Warsaw', scope: 'builtin' },
      { name: 'MOVIES', value: 'Media → /Movies', scope: 'global' },
      { name: 'DOWNLOADS', value: 'Backups → /downloads', scope: 'global' },
    ],
    storage: [
      { containerPath: '/config', resourceId: 'apps', resourceLabel: 'Apps SSD / radarr' },
      { containerPath: '/movies', resourceId: 'share-media', resourceLabel: 'Media (share)' },
    ],
    ports: [{ host: 7878, container: 7878, label: 'Web UI' }],
    risks: [],
    cpuPercent: 2,
    ramUsedBytes: 420 * MB,
    restarts: 0,
    lastDeploy: daysAgo(20),
    backup: { strategy: 'stop-backup', lastBackupAt: daysAgo(5), appdataSizeBytes: 290 * MB },
    recoveryCoverage: 83,
  },
  {
    id: 'stack-nextcloud',
    name: 'nextcloud',
    catalogId: 'nextcloud',
    category: 'Files',
    status: 'healthy',
    state: 'running',
    images: ['nextcloud:29.0.4-apache', 'postgres:16-alpine', 'redis:7.4-alpine'],
    composeYaml: NEXTCLOUD_COMPOSE,
    env: [
      { name: 'TRUSTED_DOMAINS', value: 'lumo-one.local', scope: 'stack' },
      { name: 'DB_PASSWORD', value: 'secret:nextcloud-db', scope: 'secret' },
      { name: 'NC_DATA', value: 'Cloud → /', scope: 'global' },
    ],
    storage: [
      { containerPath: '/var/www/html/data', resourceId: 'share-cloud', resourceLabel: 'Cloud (share)' },
      { containerPath: '/var/www/html', resourceId: 'apps', resourceLabel: 'Apps SSD / nextcloud' },
    ],
    ports: [{ host: 8080, container: 80, label: 'Web UI' }],
    risks: [],
    cpuPercent: 2,
    ramUsedBytes: 1.1 * GB,
    restarts: 2,
    lastDeploy: daysAgo(40),
    backup: { strategy: 'crash-consistent', lastBackupAt: hoursAgo(12), appdataSizeBytes: 3.4 * GB },
    recoveryCoverage: 91,
  },
]

export const containers: DockerContainer[] = [
  {
    id: 'ctr-jellyfin',
    name: 'jellyfin',
    stackId: 'stack-jellyfin',
    image: 'jellyfin/jellyfin:10.10.6',
    state: 'running',
    cpuPercent: 4,
    ramUsedBytes: 1.2 * GB,
    restarts: 0,
    ports: [{ host: 8096, container: 8096 }],
    startedAt: daysAgo(12),
  },
  {
    id: 'ctr-immich-server',
    name: 'immich-server',
    stackId: 'stack-immich',
    image: 'ghcr.io/immich-app/immich-server:v1.116.0',
    state: 'running',
    cpuPercent: 3,
    ramUsedBytes: 980 * MB,
    restarts: 0,
    ports: [{ host: 2283, container: 2283 }],
    startedAt: daysAgo(6),
  },
  {
    id: 'ctr-immich-ml',
    name: 'immich-machine-learning',
    stackId: 'stack-immich',
    image: 'ghcr.io/immich-app/immich-machine-learning:v1.116.0',
    state: 'running',
    cpuPercent: 2,
    ramUsedBytes: 1.6 * GB,
    restarts: 1,
    ports: [],
    startedAt: daysAgo(6),
  },
  {
    id: 'ctr-immich-db',
    name: 'immich-db',
    stackId: 'stack-immich',
    image: 'tensorchord/pgvecto-rs:pg14-v0.2.0',
    state: 'running',
    cpuPercent: 1,
    ramUsedBytes: 210 * MB,
    restarts: 0,
    ports: [],
    startedAt: daysAgo(6),
  },
  {
    id: 'ctr-immich-redis',
    name: 'immich-redis',
    stackId: 'stack-immich',
    image: 'redis:7.4-alpine',
    state: 'running',
    cpuPercent: 0,
    ramUsedBytes: 34 * MB,
    restarts: 0,
    ports: [],
    startedAt: daysAgo(6),
  },
  {
    id: 'ctr-homeassistant',
    name: 'homeassistant',
    stackId: 'stack-homeassistant',
    image: 'ghcr.io/home-assistant/home-assistant:stable',
    state: 'running',
    cpuPercent: 6,
    ramUsedBytes: 940 * MB,
    restarts: 0,
    ports: [{ host: 8123, container: 8123 }],
    startedAt: daysAgo(30),
  },
  {
    id: 'ctr-pihole',
    name: 'pihole',
    stackId: 'stack-pihole',
    image: 'pihole/pihole:v6.0',
    state: 'unhealthy',
    cpuPercent: 1,
    ramUsedBytes: 180 * MB,
    restarts: 3,
    ports: [
      { host: 80, container: 80 },
      { host: 53, container: 53 },
    ],
    startedAt: daysAgo(2),
  },
  {
    id: 'ctr-sonarr',
    name: 'sonarr',
    stackId: 'stack-sonarr',
    image: 'linuxserver/sonarr:4.0.11',
    state: 'exited',
    cpuPercent: 0,
    ramUsedBytes: 0,
    restarts: 0,
    ports: [{ host: 8989, container: 8989 }],
  },
  {
    id: 'ctr-radarr',
    name: 'radarr',
    stackId: 'stack-radarr',
    image: 'linuxserver/radarr:5.15.1',
    state: 'running',
    cpuPercent: 2,
    ramUsedBytes: 420 * MB,
    restarts: 0,
    ports: [{ host: 7878, container: 7878 }],
    startedAt: daysAgo(20),
  },
  {
    id: 'ctr-nextcloud-app',
    name: 'nextcloud-app',
    stackId: 'stack-nextcloud',
    image: 'nextcloud:29.0.4-apache',
    state: 'running',
    cpuPercent: 2,
    ramUsedBytes: 860 * MB,
    restarts: 2,
    ports: [{ host: 8080, container: 80 }],
    startedAt: daysAgo(40),
  },
  {
    id: 'ctr-nextcloud-db',
    name: 'nextcloud-db',
    stackId: 'stack-nextcloud',
    image: 'postgres:16-alpine',
    state: 'running',
    cpuPercent: 1,
    ramUsedBytes: 240 * MB,
    restarts: 0,
    ports: [],
    startedAt: daysAgo(40),
  },
  {
    id: 'ctr-nextcloud-redis',
    name: 'nextcloud-redis',
    stackId: 'stack-nextcloud',
    image: 'redis:7.4-alpine',
    state: 'running',
    cpuPercent: 0,
    ramUsedBytes: 18 * MB,
    restarts: 0,
    ports: [],
    startedAt: daysAgo(40),
  },
]

export const images: DockerImage[] = [
  { id: 'img-jf', repo: 'jellyfin/jellyfin', tag: '10.10.6', sizeBytes: 1.1 * GB, createdDaysAgo: 12, updateAvailable: true, inUse: true },
  { id: 'img-jf-old', repo: 'jellyfin/jellyfin', tag: '10.10.5', sizeBytes: 1.1 * GB, createdDaysAgo: 40, updateAvailable: false, inUse: false },
  { id: 'img-immich', repo: 'ghcr.io/immich-app/immich-server', tag: 'v1.116.0', sizeBytes: 480 * MB, createdDaysAgo: 6, updateAvailable: false, inUse: true },
  { id: 'img-immich-ml', repo: 'ghcr.io/immich-app/immich-machine-learning', tag: 'v1.116.0', sizeBytes: 2.4 * GB, createdDaysAgo: 6, updateAvailable: false, inUse: true },
  { id: 'img-pgvector', repo: 'tensorchord/pgvecto-rs', tag: 'pg14-v0.2.0', sizeBytes: 380 * MB, createdDaysAgo: 60, updateAvailable: false, inUse: true },
  { id: 'img-redis', repo: 'redis', tag: '7.4-alpine', sizeBytes: 42 * MB, createdDaysAgo: 60, updateAvailable: false, inUse: true },
  { id: 'img-ha', repo: 'ghcr.io/home-assistant/home-assistant', tag: 'stable', sizeBytes: 2.1 * GB, createdDaysAgo: 2, updateAvailable: false, inUse: true },
  { id: 'img-pihole', repo: 'pihole/pihole', tag: 'v6.0', sizeBytes: 320 * MB, createdDaysAgo: 2, updateAvailable: false, inUse: true },
  { id: 'img-sonarr', repo: 'linuxserver/sonarr', tag: '4.0.11', sizeBytes: 340 * MB, createdDaysAgo: 20, updateAvailable: false, inUse: true },
  { id: 'img-radarr', repo: 'linuxserver/radarr', tag: '5.15.1', sizeBytes: 310 * MB, createdDaysAgo: 20, updateAvailable: false, inUse: true },
  { id: 'img-nextcloud', repo: 'nextcloud', tag: '29.0.4-apache', sizeBytes: 940 * MB, createdDaysAgo: 40, updateAvailable: false, inUse: true },
  { id: 'img-postgres', repo: 'postgres', tag: '16-alpine', sizeBytes: 260 * MB, createdDaysAgo: 90, updateAvailable: false, inUse: true },
]

export const volumes: DockerVolume[] = [
  { id: 'vol-jf-config', name: 'jellyfin-config', stackId: 'stack-jellyfin', stackName: 'jellyfin', usedBytes: 2.1 * GB, bindPath: '/srv/lumonas/apps/jellyfin/config' },
  { id: 'vol-immich-upload', name: 'immich-upload', stackId: 'stack-immich', stackName: 'immich', usedBytes: 184 * GB, bindPath: '/srv/pools/main/Photos' },
  { id: 'vol-immich-db', name: 'immich-db', stackId: 'stack-immich', stackName: 'immich', usedBytes: 1.4 * GB, bindPath: '/srv/lumonas/apps/immich/db' },
  { id: 'vol-immich-ml', name: 'immich-ml-cache', stackId: 'stack-immich', stackName: 'immich', usedBytes: 3.2 * GB, bindPath: '/srv/lumonas/apps/immich/ml-cache' },
  { id: 'vol-ha-config', name: 'homeassistant-config', stackId: 'stack-homeassistant', stackName: 'homeassistant', usedBytes: 640 * MB, bindPath: '/srv/lumonas/apps/homeassistant/config' },
  { id: 'vol-pihole', name: 'pihole-etc', stackId: 'stack-pihole', stackName: 'pihole', usedBytes: 42 * MB, bindPath: '/srv/lumonas/apps/pihole/etc-pihole' },
  { id: 'vol-sonarr', name: 'sonarr-config', stackId: 'stack-sonarr', stackName: 'sonarr', usedBytes: 310 * MB, bindPath: '/srv/lumonas/apps/sonarr/config' },
  { id: 'vol-radarr', name: 'radarr-config', stackId: 'stack-radarr', stackName: 'radarr', usedBytes: 290 * MB, bindPath: '/srv/lumonas/apps/radarr/config' },
  { id: 'vol-nc-html', name: 'nextcloud-html', stackId: 'stack-nextcloud', stackName: 'nextcloud', usedBytes: 2.8 * GB, bindPath: '/srv/lumonas/apps/nextcloud/html' },
  { id: 'vol-nc-db', name: 'nextcloud-db', stackId: 'stack-nextcloud', stackName: 'nextcloud', usedBytes: 620 * MB, bindPath: '/srv/lumonas/apps/nextcloud/db' },
]

export const dockerDeployments: DockerDeployment[] = [
  {
    id: 'deploy-jellyfin-install',
    stackName: 'jellyfin',
    kind: 'install',
    state: 'committed',
    composeAfter: JELLYFIN_COMPOSE,
    createdAt: daysAgo(12),
    updatedAt: daysAgo(12),
  },
  {
    id: 'deploy-immich-update',
    stackName: 'immich',
    kind: 'update',
    state: 'committed',
    composeBefore: 'services:\n  immich:\n    image: ghcr.io/immich-app/immich-server:v1.108.0\n',
    composeAfter: 'services:\n  immich:\n    image: ghcr.io/immich-app/immich-server:v1.115.0\n',
    createdAt: hoursAgo(30),
    updatedAt: hoursAgo(30),
  },
  {
    id: 'deploy-pihole-update',
    stackName: 'pihole',
    kind: 'update',
    state: 'rolled_back',
    composeBefore: 'services:\n  pihole:\n    image: pihole/pihole:2024.05.0\n',
    composeAfter: 'services:\n  pihole:\n    image: pihole/pihole:2024.07.0\n',
    error: 'stack update failed its health gate: DNS resolution probe never passed',
    createdAt: daysAgo(3),
    updatedAt: daysAgo(3),
  },
]

const LOG_SEEDS: Record<string, string[]> = {
  jellyfin: [
    'Playback started: /media/Movies/Dune (2021)/Dune.mkv [direct play]',
    'Session heartbeat: 192.168.1.34 (Android TV)',
    'Library scan: Media — 3 items changed',
    'Transcoding: hevc → h264 (burn subtitles)',
    'Session ended: 192.168.1.34',
  ],
  'immich-server': [
    'GET /api/assets 200 41ms',
    'Asset upload queued: IMG_4471.HEIC (3.2 MB)',
    'GET /api/users/me 200 4ms',
    'Websocket connected: 2 clients',
    'Job thumbnail-generation completed in 812ms',
  ],
  'immich-machine-learning': [
    'model inference took 84ms (facial-recognition)',
    'model inference took 41ms (clip)',
    'loaded model: buffalo_l (38MB)',
  ],
  'immich-db': [
    'checkpoint complete: wrote 1244 buffers',
    'automatic vacuum of table "assets": index scans 1',
  ],
  'immich-redis': [
    'Ready to accept connections tcp',
    'DB saved on disk',
  ],
  homeassistant: [
    'Setup of domain sensor took 0.4 seconds',
    'Timer service restarted',
    'zigbee device joined: 0x00158d000a4b1f2c',
    'Automation "evening lights" triggered',
  ],
  pihole: [
    'error: FTL failed to bind to port 53 — address in use',
    'dnsmasq: syntax check OK',
    'gravity database updated (1_234_567 domains)',
  ],
  radarr: [
    'MovieService: Scanning disk for 812 movies',
    'Downloaded: Dune Part Two (2024) [WEBDL-1080p]',
    'Renamed 1 file using naming scheme',
  ],
  'nextcloud-app': [
    'GET /remote.php/dav/files/admin/ 207 62ms',
    'background job: cleanup file trashbin (retention 30d)',
    'GET /index.php/login 200 11ms',
  ],
  'nextcloud-db': [
    'checkpoint complete: wrote 318 buffers',
  ],
  'nextcloud-redis': [
    'Ready to accept connections tcp',
  ],
}

export function logSeedsFor(container: string): string[] {
  return LOG_SEEDS[container] ?? ['service started', 'ready', 'idle']
}

export function seedLogs(container: string): LogLine[] {
  const seeds = logSeedsFor(container)
  return seeds.map((message, i) => ({
    container,
    ts: new Date(now - (seeds.length - i) * 7_000).toISOString(),
    level: message.startsWith('error') ? 'error' : 'info',
    message,
  }))
}

export function findStack(id: string): DockerStack | undefined {
  return stacks.find((s) => s.id === id)
}

export function findContainer(id: string): DockerContainer | undefined {
  return containers.find((c) => c.id === id)
}
