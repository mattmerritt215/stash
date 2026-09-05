PRAGMA foreign_keys=OFF;

-- ---------------------------------------------------------------------------
-- users
-- ---------------------------------------------------------------------------
-- Row 1 is the local administrator. It owns every piece of per-user state
-- that existed before this migration, so a single-user install upgrades
-- without losing ratings, favourites, play history or saved filters.

CREATE TABLE `users` (
  `id`            integer not null primary key autoincrement,
  `username`      varchar(255) not null,
  `subject`       varchar(255),
  `email`         varchar(255),
  `display_name`  varchar(255),
  `role`          varchar(32) not null default 'user',
  `disabled`      boolean not null default '0',
  `api_key_hash`  varchar(64),
  `ui_config`     text not null default '{}',
  `created_at`    datetime not null,
  `updated_at`    datetime not null,
  `last_login_at` datetime
);

CREATE UNIQUE INDEX `index_users_on_username_unique` ON `users` (`username` COLLATE NOCASE);
CREATE UNIQUE INDEX `index_users_on_subject_unique`  ON `users` (`subject`) WHERE `subject` IS NOT NULL;
CREATE UNIQUE INDEX `index_users_on_api_key_unique`  ON `users` (`api_key_hash`) WHERE `api_key_hash` IS NOT NULL;

INSERT INTO `users` (`id`, `username`, `role`, `disabled`, `ui_config`, `created_at`, `updated_at`)
VALUES (1, 'admin', 'admin', '0', '{}', datetime('now'), datetime('now'));

-- ---------------------------------------------------------------------------
-- saved filters become per-user
-- ---------------------------------------------------------------------------

CREATE TABLE `saved_filters_new` (
  `id`            integer not null primary key autoincrement,
  `user_id`       integer not null default 1,
  `name`          varchar(510) not null,
  `mode`          varchar(255) not null,
  `find_filter`   blob,
  `object_filter` blob,
  `ui_options`    blob,
  foreign key(`user_id`) references `users`(`id`) on delete CASCADE
);

INSERT INTO `saved_filters_new`
  (`id`, `user_id`, `name`, `mode`, `find_filter`, `object_filter`, `ui_options`)
  SELECT `id`, 1, `name`, `mode`, `find_filter`, `object_filter`, `ui_options`
  FROM `saved_filters`;

DROP INDEX IF EXISTS `index_saved_filters_on_mode_name_unique`;
DROP TABLE `saved_filters`;
ALTER TABLE `saved_filters_new` RENAME TO `saved_filters`;

CREATE UNIQUE INDEX `index_saved_filters_on_user_mode_name_unique`
  ON `saved_filters` (`user_id`, `mode`, `name`);
CREATE INDEX `index_saved_filters_on_user_id` ON `saved_filters` (`user_id`);

-- ---------------------------------------------------------------------------
-- activity history becomes per-user
-- ---------------------------------------------------------------------------

ALTER TABLE `scenes_view_dates` ADD COLUMN `user_id` integer not null default 1
  REFERENCES `users`(`id`) ON DELETE CASCADE;
ALTER TABLE `scenes_o_dates`    ADD COLUMN `user_id` integer not null default 1
  REFERENCES `users`(`id`) ON DELETE CASCADE;

CREATE INDEX `index_scenes_view_dates_on_user_scene` ON `scenes_view_dates` (`user_id`, `scene_id`);
CREATE INDEX `index_scenes_o_dates_on_user_scene`    ON `scenes_o_dates` (`user_id`, `scene_id`);

-- images still carry a plain o_counter column upstream; give it a per-user
-- history table of the same shape as the scene one.
CREATE TABLE `images_o_dates` (
  `image_id` integer not null,
  `user_id`  integer not null default 1,
  `o_date`   datetime not null,
  foreign key(`image_id`) references `images`(`id`) on delete CASCADE,
  foreign key(`user_id`)  references `users`(`id`)  on delete CASCADE
);
CREATE INDEX `index_images_o_dates_on_user_image` ON `images_o_dates` (`user_id`, `image_id`);

INSERT INTO `images_o_dates` (`image_id`, `user_id`, `o_date`)
WITH RECURSIVE numbers(n) AS (
  SELECT 1
  UNION ALL
  SELECT n + 1 FROM numbers WHERE n < (SELECT COALESCE(MAX(o_counter), 0) FROM images)
)
SELECT images.id, 1, images.created_at
FROM images JOIN numbers ON numbers.n <= images.o_counter;

-- ---------------------------------------------------------------------------
-- per-user overlay state
-- ---------------------------------------------------------------------------
-- The base tables keep their rating/favorite/organized columns untouched, so
-- upstream code and upstream migrations continue to work. These overlay tables
-- hold each user's own value; a NULL or missing row means "fall back to the
-- base column", which is what makes the migration lossless for user 1.

CREATE TABLE `scenes_user_state` (
  `scene_id`      integer not null,
  `user_id`       integer not null,
  `rating`        tinyint,
  `organized`     boolean,
  `resume_time`   float,
  `play_duration` float,
  `updated_at`    datetime not null default (datetime('now')),
  primary key (`scene_id`, `user_id`),
  foreign key(`scene_id`) references `scenes`(`id`) on delete CASCADE,
  foreign key(`user_id`)  references `users`(`id`)  on delete CASCADE
);


CREATE TABLE `images_user_state` (
  `image_id`   integer not null,
  `user_id`    integer not null,
  `rating`     tinyint,
  `organized`  boolean,
  `updated_at` datetime not null default (datetime('now')),
  primary key (`image_id`, `user_id`),
  foreign key(`image_id`) references `images`(`id`) on delete CASCADE,
  foreign key(`user_id`)  references `users`(`id`)  on delete CASCADE
);


CREATE TABLE `galleries_user_state` (
  `gallery_id` integer not null,
  `user_id`    integer not null,
  `rating`     tinyint,
  `organized`  boolean,
  `updated_at` datetime not null default (datetime('now')),
  primary key (`gallery_id`, `user_id`),
  foreign key(`gallery_id`) references `galleries`(`id`) on delete CASCADE,
  foreign key(`user_id`)    references `users`(`id`)     on delete CASCADE
);


CREATE TABLE `performers_user_state` (
  `performer_id` integer not null,
  `user_id`      integer not null,
  `rating`       tinyint,
  `favorite`     boolean,
  `updated_at`   datetime not null default (datetime('now')),
  primary key (`performer_id`, `user_id`),
  foreign key(`performer_id`) references `performers`(`id`) on delete CASCADE,
  foreign key(`user_id`)      references `users`(`id`)      on delete CASCADE
);


CREATE TABLE `studios_user_state` (
  `studio_id`  integer not null,
  `user_id`    integer not null,
  `rating`     tinyint,
  `favorite`   boolean,
  `organized`  boolean,
  `updated_at` datetime not null default (datetime('now')),
  primary key (`studio_id`, `user_id`),
  foreign key(`studio_id`) references `studios`(`id`) on delete CASCADE,
  foreign key(`user_id`)   references `users`(`id`)   on delete CASCADE
);


CREATE TABLE `tags_user_state` (
  `tag_id`     integer not null,
  `user_id`    integer not null,
  `favorite`   boolean,
  `updated_at` datetime not null default (datetime('now')),
  primary key (`tag_id`, `user_id`),
  foreign key(`tag_id`)  references `tags`(`id`)  on delete CASCADE,
  foreign key(`user_id`) references `users`(`id`) on delete CASCADE
);


CREATE TABLE `groups_user_state` (
  `group_id`   integer not null,
  `user_id`    integer not null,
  `rating`     tinyint,
  `updated_at` datetime not null default (datetime('now')),
  primary key (`group_id`, `user_id`),
  foreign key(`group_id`) references `groups`(`id`) on delete CASCADE,
  foreign key(`user_id`)  references `users`(`id`)  on delete CASCADE
);


PRAGMA foreign_keys=ON;
