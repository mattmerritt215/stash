import React from "react";
import cx from "classnames";
import { Button, Spinner } from "react-bootstrap";
import { Icon } from "src/components/Shared/Icon";
import { defineMessages, useIntl } from "react-intl";
import { faDice } from "@fortawesome/free-solid-svg-icons";

export interface IRandomExcludedButtonProps {
  loading: boolean;
  excluded: boolean;
  onClick: () => void;
}

export const RandomExcludedButton: React.FC<IRandomExcludedButtonProps> = (
  props: IRandomExcludedButtonProps
) => {
  const intl = useIntl();
  const messages = defineMessages({
    randomExcluded: {
      id: "random_excluded",
      defaultMessage: "Excluded from random",
    },
    randomIncluded: {
      id: "random_included",
      defaultMessage: "Included in random",
    },
  });

  if (props.loading) return <Spinner animation="border" role="status" />;

  return (
    <Button
      variant="secondary"
      title={intl.formatMessage(
        props.excluded ? messages.randomExcluded : messages.randomIncluded
      )}
      className={cx(
        "minimal",
        "random-excluded-button",
        props.excluded ? "random-excluded" : "not-random-excluded"
      )}
      onClick={props.onClick}
    >
      <Icon icon={faDice} />
    </Button>
  );
};
