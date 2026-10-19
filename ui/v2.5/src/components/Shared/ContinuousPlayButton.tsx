import React, { useState } from "react";
import { Button, Spinner } from "react-bootstrap";
import { useHistory } from "react-router-dom";
import { defineMessages, useIntl } from "react-intl";
import cx from "classnames";
import { faPlay } from "@fortawesome/free-solid-svg-icons";
import { Icon } from "./Icon";
import { useToast } from "src/hooks/Toast";
import {
  ContinuousPlayCategory,
  startContinuousPlay,
} from "src/utils/continuousPlay";

const messages = defineMessages({
  continuousPlay: {
    id: "continuous_play",
    defaultMessage: "Start Continuous Play",
  },
  continuousPlayEmpty: {
    id: "continuous_play_empty",
    defaultMessage: "No scenes available to play.",
  },
});

export interface IContinuousPlayButtonProps {
  category: ContinuousPlayCategory;
  className?: string;
}

export const ContinuousPlayButton: React.FC<IContinuousPlayButtonProps> = ({
  category,
  className,
}) => {
  const intl = useIntl();
  const history = useHistory();
  const Toast = useToast();
  const [loading, setLoading] = useState(false);

  async function onClick(e: React.MouseEvent) {
    e.preventDefault();
    e.stopPropagation();
    if (loading) return;

    setLoading(true);
    try {
      const link = await startContinuousPlay(category);
      if (!link) {
        Toast.error(intl.formatMessage(messages.continuousPlayEmpty));
        return;
      }
      history.push(link);
    } catch (err) {
      Toast.error(err);
    } finally {
      setLoading(false);
    }
  }

  if (loading) {
    return <Spinner animation="border" role="status" />;
  }

  return (
    <Button
      variant="secondary"
      title={intl.formatMessage(messages.continuousPlay)}
      className={cx("minimal", "continuous-play-button", className)}
      onClick={onClick}
    >
      <Icon icon={faPlay} />
    </Button>
  );
};
