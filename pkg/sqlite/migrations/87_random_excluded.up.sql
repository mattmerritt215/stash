-- Per-user "exclude from random" flag for scenes. This column has no
-- upstream base-table counterpart -- a missing overlay row means "not
-- excluded" (false), not "inherit the library default".
ALTER TABLE `scenes_user_state` ADD COLUMN `random_excluded` boolean;
