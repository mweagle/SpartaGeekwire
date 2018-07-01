import React from 'react';
import Box from 'grommet/components/Box';
import Header from 'grommet/components/Header';
import Menu from 'grommet/components/Menu';
import GrommetIcon from 'grommet/components/icons/base/BrandGrommetOutline';
import Heading from 'grommet/components/Heading';

export default function AppHeader (props) {
  return (
    <Header justify="left" colorIndex="neutral-3">
      <Box size={{width: {max: 'xxlarge'}}}
        direction="row"
        responsive={false}
        justify="start"
        align="center"
        pad={{horizontal: 'medium'}}
        flex="grow">
        <GrommetIcon colorIndex="brand" size="large" />
        <Box pad="small" />
        <Menu label="Label" inline={true} direction="row" flex="grow">
          <Heading strong={false}>
            Geekwire Cloud Tech Summit
          </Heading>
        </Menu>
      </Box>
    </Header>
  );
};
