import React from "react";
import { Icon } from "../Shared/Icon";
import { Button } from "react-bootstrap";
import { faDice } from "@fortawesome/free-solid-svg-icons";
import cx from "classnames";
import { SizeProp } from "@fortawesome/fontawesome-svg-core";

export const RandomExcludeIcon: React.FC<{
  excluded: boolean;
  onToggleExcluded: (v: boolean) => void;
  size?: SizeProp;
  className?: string;
}> = ({ excluded, onToggleExcluded, size, className }) => {
  return (
    <Button
      className={cx(
        "minimal",
        "mousetrap",
        "random-exclude-button",
        className,
        excluded ? "random-excluded" : "not-random-excluded"
      )}
      onClick={() => onToggleExcluded(!excluded)}
    >
      <Icon icon={faDice} size={size} />
    </Button>
  );
};
